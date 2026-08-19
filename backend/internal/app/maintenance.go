package app

// The two things the binary does that are not "run the house": reset the
// admin password, and listen to what an announcement would sound like.
//
// Both are deliberately outside App. They are run by someone setting the house
// up or getting back into it, they touch one thing each, and neither should
// pay for — or risk — assembling a server first.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"homehub/internal/announce"
	"homehub/internal/store"
)

// voiceCheckBudget caps the synthesis round trip. Generous: a cold
// text-to-speech service can take a while to answer the first request, and
// this is a one-shot command with a person waiting on it.
const voiceCheckBudget = 60 * time.Second

// voiceCheckFile is where the finished announcement is written, next to the
// binary, so the household can play it back.
const voiceCheckFile = "announcement.wav"

// ResetAdminPassword sets the first admin's password from newPass and
// invalidates every session minted with the old one. It returns the admin's
// username so the caller can say whose password just changed.
//
// This exists because the alternative — delete users.json and re-seed — throws
// away every other profile in the house along with the forgotten password.
func ResetAdminPassword(dataDir, newPass string) (string, error) {
	if newPass == "" {
		return "", errors.New("AUTH_PASS is not set — export it before running --reset-admin")
	}

	st := store.New(dataDir, nil)
	if err := st.Load(); err != nil {
		return "", fmt.Errorf("loading data: %w", err)
	}

	var username string
	err := st.Update(func() error {
		var admin *store.User
		for _, u := range st.Users {
			if u.Admin {
				admin = u
				break
			}
		}
		if admin == nil {
			return errors.New("no admin user found — delete data/users.json and restart to re-seed from AUTH_USER/AUTH_PASS")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hashing password: %w", err)
		}
		admin.PasswordHash = string(hash)
		admin.TokenVersion++ // invalidate any sessions minted with the old password
		username = admin.Username
		return nil
	})
	if err != nil {
		return "", err
	}
	return username, nil
}

// CheckVoice synthesises one phrase through the configured service and writes
// the finished announcement — chime, pause, words — next to the binary, so the
// household can listen to what the speakers would play before trusting it to
// call anyone.
//
// It runs before anything is opened: it touches no state, and someone running
// it is setting the house up rather than running it. An announcement falls
// back to the chime whenever the words can't be made, which is why a
// misconfigured endpoint is otherwise indistinguishable from none — right at
// dinner time.
//
// A chime-only house is a supported setup, not a failure, so a phrase that
// could not be spoken still returns nil. The summary says what is missing.
func CheckVoice(phrase string) error {
	ctx, cancel := context.WithTimeout(context.Background(), voiceCheckBudget)
	defer cancel()

	res := announce.Check(ctx, announce.VoiceFromEnv(), phrase)
	fmt.Print(res.Summary())

	if err := os.WriteFile(voiceCheckFile, res.Clip.WAV(), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", voiceCheckFile, err)
	}
	fmt.Printf("  written to %s — play it to hear what the rooms would.\n", voiceCheckFile)
	return nil
}
