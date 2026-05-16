// Package rx runs a 433MHz receiver subprocess and feeds the decoded
// packets into the sensor store as readings.
//
// The subprocess is expected to emit one JSON object per line on stdout
// (the "rtl_433 -F json" convention). Each line is matched against the
// configured sensors by (protocol, code); when a sensor matches, the
// numeric value at sensor.Field is recorded as a reading.
//
// In dev environments without a receiver the listener simply logs and
// exits — readings can still be ingested via POST /sensors/{id}/readings.
package rx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"rf-socket-controller/internal/store"
)

// Listener runs an external 433MHz receiver and dispatches decoded
// packets to the store.
type Listener struct {
	// Command + args to spawn. The process must print one JSON object
	// per line on stdout. Defaults to ["rtl_433", "-F", "json"] when
	// the SENSOR_RX_CMD env var is unset.
	Command []string
}

// FromEnv returns a Listener configured from SENSOR_RX_CMD (space-split)
// or the default rtl_433 invocation.
func FromEnv() Listener {
	if v := strings.TrimSpace(os.Getenv("SENSOR_RX_CMD")); v != "" {
		return Listener{Command: strings.Fields(v)}
	}
	return Listener{Command: []string{"rtl_433", "-F", "json"}}
}

// Run blocks until ctx is cancelled. It spawns the receiver subprocess
// and re-spawns it on exit with a short backoff. Spawn it in a goroutine.
func (l Listener) Run(ctx context.Context, st *store.Store) {
	if len(l.Command) == 0 {
		log.Printf("rx: no command configured — RX listener disabled")
		return
	}
	if _, err := exec.LookPath(l.Command[0]); err != nil {
		log.Printf("rx: %q not found in PATH — RX listener disabled "+
			"(install rtl_433 or set SENSOR_RX_CMD to your decoder)", l.Command[0])
		return
	}

	log.Printf("rx: starting receiver: %s", strings.Join(l.Command, " "))
	backoff := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := l.runOnce(ctx, st); err != nil {
			log.Printf("rx: receiver exited: %v (restarting in %s)", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (l Listener) runOnce(ctx context.Context, st *store.Store) error {
	cmd := exec.CommandContext(ctx, l.Command[0], l.Command[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var packet map[string]interface{}
		if err := json.Unmarshal(line, &packet); err != nil {
			continue
		}
		l.dispatch(st, packet)
	}
	return cmd.Wait()
}

// dispatch maps an incoming packet to a sensor and records a reading.
//
// Matching rules:
//   - protocol comes from the packet's "model" field (rtl_433's convention)
//   - code comes from the packet's "id" field, joined to model as "model:id"
//     if the user prefers that form, OR matched directly against the "id" field
//   - field tells us which JSON key to read as the value; if empty, we
//     pick the first numeric field that isn't a known identifier
func (l Listener) dispatch(st *store.Store, packet map[string]interface{}) {
	model, _ := packet["model"].(string)
	id := stringifyID(packet["id"])
	composed := model
	if id != "" {
		composed = model + ":" + id
	}

	st.Mu.Lock()
	defer st.Mu.Unlock()

	matched := false
	for _, sensor := range st.Sensors {
		if !codeMatches(sensor.Code, composed, model, id) {
			continue
		}
		if sensor.Protocol != "" && !strings.EqualFold(sensor.Protocol, model) &&
			!strings.EqualFold(sensor.Protocol, "rtl_433") {
			continue
		}
		matched = true
		value, ok := extractValue(packet, sensor.Field)
		if !ok {
			continue
		}
		reading := store.SensorReading{Time: time.Now().UTC(), Value: value}
		if err := st.AppendReading(sensor.ID, reading); err != nil {
			log.Printf("rx: append reading for %s: %v", sensor.ID, err)
		}
	}

	if !matched && composed != "" && st.DiscoveryActive() {
		st.RecordCandidate(model, composed, numericFields(packet))
	}
}

// numericFields returns every JSON key that decodes to a number,
// skipping known identifier fields. Used by discovery so the UI can
// show the user which numeric value each candidate is producing.
func numericFields(packet map[string]interface{}) map[string]float64 {
	out := make(map[string]float64, len(packet))
	for k, v := range packet {
		if isIdentifierField(k) {
			continue
		}
		if f, ok := toFloat(v); ok {
			out[k] = f
		}
	}
	return out
}

func codeMatches(want, composed, model, id string) bool {
	if want == "" {
		return false
	}
	if want == composed || want == model || want == id {
		return true
	}
	return false
}

func stringifyID(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%d", int64(x))
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// extractValue reads the requested field as a float64. When field is
// empty it returns the first numeric field that isn't a known identifier,
// which covers simple "one number per packet" sensors.
func extractValue(packet map[string]interface{}, field string) (float64, bool) {
	if field != "" {
		v, ok := packet[field]
		if !ok {
			return 0, false
		}
		return toFloat(v)
	}
	for k, v := range packet {
		if isIdentifierField(k) {
			continue
		}
		if f, ok := toFloat(v); ok {
			return f, true
		}
	}
	return 0, false
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func isIdentifierField(k string) bool {
	switch strings.ToLower(k) {
	case "id", "model", "time", "channel", "subtype", "protocol", "device":
		return true
	}
	return false
}
