package matter

import (
	"context"
	"log"
	"strings"
	"time"

	"homehub/internal/store"
)

// pollInterval controls how often commissioned Matter sensors are read and
// recorded as a reading. Wider than the rtl_433/MQTT paths: those are
// passive listeners, but reading a Matter cluster is a live round trip to
// the bridge (and, on first reach after idle, a fresh CASE session), so
// polling every device on their cadence would generate needless traffic.
const pollInterval = 60 * time.Second

// SensorPoller reads the TemperatureMeasurement / RelativeHumidityMeasurement
// clusters of every store.Sensor whose Protocol is "matter" and records the
// value as a reading. Matter sensors have no push path into this backend
// (the bridge doesn't do subscription reports, only on-demand reads), so —
// unlike rx.Listener and mqtt.SensorListener — this polls instead of
// listening.
//
// For a Matter sensor:
//   - Code is the commissioned node id (as returned by /matter/commission,
//     the same id a Socket would store there for a light or plug).
//   - Field selects which cluster to read: "temperature" or "humidity".
//     Empty falls back to Kind ("humidity" reads the humidity cluster,
//     anything else reads temperature).
type SensorPoller struct {
	Client *Client
}

// Run blocks until ctx is cancelled. Spawn it in a goroutine. A nil/disabled
// Client makes this a no-op, matching Enabled()'s use elsewhere.
func (p SensorPoller) Run(ctx context.Context, st *store.Store) {
	if !p.Client.Enabled() {
		return
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	p.sweep(ctx, st)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sweep(ctx, st)
		}
	}
}

// matterSensor is a snapshot of the fields needed to poll one sensor, taken
// under the store lock so the bridge round trip below runs lock-free.
type matterSensor struct {
	id    string
	code  string
	field string
	kind  string
}

func (p SensorPoller) sweep(ctx context.Context, st *store.Store) {
	var sensors []matterSensor
	st.View(func() {
		for _, sn := range st.Sensors {
			if !strings.EqualFold(sn.Protocol, "matter") || sn.Code == "" {
				continue
			}
			sensors = append(sensors, matterSensor{id: sn.ID, code: sn.Code, field: sn.Field, kind: sn.Kind})
		}
	})

	for _, sn := range sensors {
		value, ok := p.read(ctx, sn)
		if !ok {
			continue
		}
		reading := store.SensorReading{Time: time.Now().UTC(), Value: value}
		st.Mutate(func() {
			if err := st.AppendReading(sn.id, reading); err != nil {
				log.Printf("matter: append reading for %s: %v", sn.id, err)
			}
		})
	}
}

// read fetches one sensor's live state from the bridge and picks the field
// its Sensor config asks for.
func (p SensorPoller) read(ctx context.Context, sn matterSensor) (float64, bool) {
	cctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	state, err := p.Client.GetState(cctx, sn.code)
	if err != nil {
		log.Printf("matter: poll sensor %s (node %s): %v", sn.id, sn.code, err)
		return 0, false
	}
	if sensorField(sn.field, sn.kind) == "humidity" {
		if state.Humidity == nil {
			return 0, false
		}
		return *state.Humidity, true
	}
	if state.Temperature == nil {
		return 0, false
	}
	return *state.Temperature, true
}

// sensorField resolves which cluster to read: an explicit Field wins,
// otherwise it's derived from Kind.
func sensorField(field, kind string) string {
	f := strings.ToLower(strings.TrimSpace(field))
	if f == "humidity" || f == "temperature" {
		return f
	}
	if strings.EqualFold(kind, "humidity") {
		return "humidity"
	}
	return "temperature"
}
