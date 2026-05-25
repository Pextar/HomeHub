package rx

// serial.go – optional USB CDC ACM serial listener for the nRF52840
// FT007TH firmware bridge.
//
// Set SENSOR_SERIAL_PORT=/dev/ttyACM0 to enable.  The port is opened as a
// plain file — no baud-rate configuration is required for USB CDC ACM because
// the serial framing is virtual over USB.  Each line emitted by the firmware
// is a JSON object in the same format as rtl_433 -F json, so the existing
// dispatch() logic handles it without modification.
//
// This listener runs alongside SENSOR_RX_CMD (rtl_433 or ft007th_rx.py); both
// can be active at the same time if you have two receiver sources.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"rf-socket-controller/internal/store"
)

// SerialListener reads JSON sensor packets from a serial / USB-CDC port.
type SerialListener struct {
	Port string // e.g. "/dev/ttyACM0"
}

// SerialFromEnv returns a SerialListener when SENSOR_SERIAL_PORT is set,
// or nil if the variable is absent.
func SerialFromEnv() *SerialListener {
	port := strings.TrimSpace(os.Getenv("SENSOR_SERIAL_PORT"))
	if port == "" {
		return nil
	}
	return &SerialListener{Port: port}
}

// Run blocks until ctx is cancelled.  It opens the port and re-opens it on
// error with a short backoff.  Call it in a goroutine.
func (s *SerialListener) Run(ctx context.Context, st *store.Store) {
	log.Printf("rx/serial: bridge enabled on %s", s.Port)
	backoff := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.runOnce(ctx, st); err != nil {
			log.Printf("rx/serial: %v (reopening in %s)", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (s *SerialListener) runOnce(ctx context.Context, st *store.Store) error {
	// USB CDC ACM on Linux can be opened like a regular file.
	// No termios / baud-rate setup is needed — the kernel handles USB framing.
	f, err := os.Open(s.Port)
	if err != nil {
		return fmt.Errorf("open %s: %w", s.Port, err)
	}
	log.Printf("rx/serial: opened %s", s.Port)

	// Reuse the dispatch() method from Listener; Command is unused there.
	l := Listener{}

	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 4096), 4096)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 || line[0] != '{' {
				continue // skip blank lines and firmware log lines starting with '#'
			}
			var packet map[string]interface{}
			if err := json.Unmarshal(line, &packet); err != nil {
				continue
			}
			l.dispatch(st, packet)
		}
		errCh <- scanner.Err()
	}()

	select {
	case <-ctx.Done():
		f.Close()
		return nil
	case err := <-errCh:
		f.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", s.Port, err)
		}
		return fmt.Errorf("port %s closed unexpectedly", s.Port)
	}
}
