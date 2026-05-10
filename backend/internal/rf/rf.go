// Package rf abstracts the userspace 433MHz transmitter so the rest of
// the backend stays platform-independent. In dev / non-Pi environments
// where no transmitter binary is installed, Send falls back to logging.
package rf

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// Sender emits 433MHz codes via whichever userspace tool is installed.
// It is safe for concurrent use; each Send shells out independently.
type Sender struct{}

// Send transmits the code with a hard timeout so a stuck transmitter
// driver cannot block a caller indefinitely. The state argument is only
// used for diagnostic logging in simulation mode.
func (Sender) Send(code, protocol string, state bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
