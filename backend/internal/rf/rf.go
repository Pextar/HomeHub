// Package rf abstracts the userspace 433MHz transmitter so the rest of
// the backend stays platform-independent. In dev / non-Pi environments
// where no transmitter is available, Send falls back to logging.
package rf

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Sender emits 433MHz codes via whichever userspace tool is available.
//
// For the Nexa self-learning protocol it shells out to NexaScript (an
// lgpio-backed Python helper); for other protocols it falls back to the
// rpi-rf_send / codesend CLIs, or to logging when nothing is installed.
// It is safe for concurrent use; each Send shells out independently.
type Sender struct {
	NexaScript string // path to nexa_tx.py; empty disables the Nexa path
}

// Send transmits the code with a hard timeout so a stuck transmitter
// cannot block a caller indefinitely. The state argument selects on/off
// for protocols that encode it (Nexa) and is otherwise diagnostic only.
func (s Sender) Send(code, protocol string, state bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if strings.EqualFold(protocol, "nexa") {
		return s.sendNexa(ctx, code, state)
	}

	var cmd *exec.Cmd
	switch {
	case lookPath("rpi-rf_send"):
		cmd = exec.CommandContext(ctx, "rpi-rf_send", code)
	case lookPath("codesend"):
		cmd = exec.CommandContext(ctx, "codesend", code)
	default:
		log.Printf("[simulation] code=%s protocol=%s state=%v", code, protocol, state)
		return nil
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", cmd.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sendNexa parses a "<houseID>:<unit>" code and invokes the Python
// transmitter. When no script is configured (e.g. dev on a laptop) it
// logs instead, mirroring the simulation fallback above.
func (s Sender) sendNexa(ctx context.Context, code string, state bool) error {
	house, unit, err := parseNexaCode(code)
	if err != nil {
		return err
	}
	arg := "off"
	if state {
		arg = "on"
	}
	if s.NexaScript == "" {
		log.Printf("[simulation] nexa house=%d unit=%d state=%v", house, unit, state)
		return nil
	}
	cmd := exec.CommandContext(ctx, "python3", s.NexaScript,
		strconv.Itoa(house), strconv.Itoa(unit), arg)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nexa_tx: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseNexaCode splits a "<houseID>:<unit>" code into its parts and
// range-checks them against the protocol's 26-bit / 4-bit fields.
func parseNexaCode(code string) (house, unit int, err error) {
	parts := strings.SplitN(strings.TrimSpace(code), ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid nexa code %q (want \"<houseID>:<unit>\")", code)
	}
	if house, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, fmt.Errorf("invalid nexa house id %q: %w", parts[0], err)
	}
	if unit, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, fmt.Errorf("invalid nexa unit %q: %w", parts[1], err)
	}
	if house < 0 || house >= (1<<26) {
		return 0, 0, fmt.Errorf("nexa house id %d out of range (0..67108863)", house)
	}
	if unit < 0 || unit > 15 {
		return 0, 0, fmt.Errorf("nexa unit %d out of range (0..15)", unit)
	}
	return house, unit, nil
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
