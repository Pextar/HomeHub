// Command homehub runs the HomeHub server.
//
// It is deliberately thin. Parsing flags and choosing between the three things
// the binary can do is all that belongs in a main package; how the server is
// assembled lives in internal/app, where it can be read in one sitting and
// built in a test.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"homehub/internal/app"
)

func main() {
	resetAdmin := flag.Bool("reset-admin", false, "reset the first admin's password from AUTH_PASS and exit")
	checkVoice := flag.String("check-voice", "",
		"synthesise a phrase through the configured text-to-speech service, write it to announcement.wav, and exit")
	flag.Parse()

	cfg := app.FromEnv()

	switch {
	case *checkVoice != "":
		if err := app.CheckVoice(*checkVoice); err != nil {
			log.Fatal(err)
		}
	case *resetAdmin:
		username, err := app.ResetAdminPassword(cfg.DataDir, os.Getenv("AUTH_PASS"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Password reset for admin %q — you can now log in with the new AUTH_PASS.\n", username)
	default:
		if err := run(cfg); err != nil {
			log.Fatal(err)
		}
	}
}

// run assembles the server and hands it a context that a second interrupt
// cannot rush: NotifyContext cancels on the first signal, and the shutdown
// sequence owns its own deadlines from there.
func run(cfg app.Config) error {
	a, err := app.New(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.Run(ctx)
}
