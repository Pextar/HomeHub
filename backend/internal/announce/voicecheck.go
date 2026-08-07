package announce

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Checking a voice service before a house depends on it.
//
// The failure this exists for is a quiet one. An announcement degrades to the
// chime whenever the words can't be made — that is the right behaviour at
// dinner time, but it means a misconfigured endpoint is indistinguishable
// from no endpoint at all: the panel chimes, the kids come, and nobody finds
// out for weeks that the sentence never played. So the setup step gets a
// check that says which of the several possible things is wrong, in the words
// that fix it.

// CheckResult is what a voice service answered, and what that means.
type CheckResult struct {
	Dialect Dialect
	// Spoke is the whole point: words came back, not just the chime.
	Spoke bool
	// Format and Speech describe what the service returned.
	Format Format
	Speech time.Duration
	// Clip is the finished announcement — chime, pause, words — so the
	// caller can write it somewhere audible. A house should hear this
	// before it trusts it.
	Clip Clip
	// Err is why there are no words, when there are none.
	Err error
	// Advice names the fix, when there is one to name.
	Advice string
}

// Check synthesises one phrase and reports what came back. It runs the same
// path an announcement does — deliberately: a check that exercised a
// different code path could pass while dinner still failed.
func Check(ctx context.Context, v *Voice, phrase string) CheckResult {
	if strings.TrimSpace(phrase) == "" {
		phrase = "Dinner's ready"
	}
	res := CheckResult{}
	if v == nil || v.URL == "" {
		res.Err = ErrNoVoice
		res.Advice = "Set HOMEHUB_TTS_URL to a text-to-speech endpoint. Without one, announcements are the chime alone."
		res.Clip = chime(defaultFormat)
		return res
	}
	res.Dialect = v.Dialect

	start := time.Now()
	speech, err := v.Speak(ctx, phrase)
	elapsed := time.Since(start)

	if err != nil {
		res.Err = err
		res.Advice = adviceFor(v, err)
		res.Clip = chime(defaultFormat)
		return res
	}

	res.Spoke = true
	res.Format = speech.Format
	res.Speech = speech.Duration()
	res.Clip = join(
		chime(speech.Format),
		Clip{Format: speech.Format, PCM: silence(speech.Format, leadIn)},
		speech,
	)
	// Slower than the audio it produced is a service that will make someone
	// wait at the wall — worth saying even though it works.
	if elapsed > res.Speech {
		res.Advice = fmt.Sprintf(
			"Synthesis took %s for %s of speech. It works, but the wall waits that long before the chime plays.",
			elapsed.Round(10*time.Millisecond), res.Speech.Round(10*time.Millisecond))
	}
	return res
}

// adviceFor turns a failure into the sentence that fixes it. The three that
// actually happen are a wrong dialect, a format nothing here can read, and a
// service that isn't there.
func adviceFor(v *Voice, err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedAudio):
		if v.Dialect == DialectOpenAI {
			return "The service answered with audio that isn't 16-bit PCM WAV. HomeHub asks for response_format=wav; check the service supports it (Kokoro-FastAPI, Speaches and openedai-speech all do)."
		}
		return "The service answered with audio that isn't 16-bit PCM WAV, which a Piper server should never do — check HOMEHUB_TTS_URL points at Piper's /synthesize and not at something else."
	case strings.Contains(err.Error(), "refused ("):
		if v.Dialect == DialectOpenAI {
			return "The service rejected the request. Check HOMEHUB_TTS_MODEL and HOMEHUB_TTS_VOICE name things it has, and HOMEHUB_TTS_KEY if it wants one."
		}
		return "The service rejected the request. Check HOMEHUB_TTS_VOICE names a voice this Piper server has downloaded, or leave it unset to use its default."
	case strings.Contains(err.Error(), "reaching the voice service"):
		return "Nothing answered at HOMEHUB_TTS_URL. Check the address, the port, and that the service is reachable from the machine HomeHub runs on."
	}
	if v.Dialect == DialectOpenAI && strings.HasSuffix(strings.TrimRight(v.URL, "/"), "/synthesize") {
		return "That URL looks like Piper's own endpoint — set HOMEHUB_TTS_KIND=piper."
	}
	return ""
}

// Summary is the check in a few lines, for a terminal.
func (r CheckResult) Summary() string {
	var b strings.Builder
	if r.Spoke {
		fmt.Fprintf(&b, "Voice OK (%s dialect)\n", r.Dialect)
		fmt.Fprintf(&b, "  speech:       %s of %d Hz, %d channel(s)\n",
			r.Speech.Round(10*time.Millisecond), r.Format.SampleRate, r.Format.Channels)
		fmt.Fprintf(&b, "  announcement: %s (chime, pause, words)\n",
			r.Clip.Duration().Round(10*time.Millisecond))
	} else {
		b.WriteString("No words — announcements would be the chime alone\n")
		if r.Err != nil {
			fmt.Fprintf(&b, "  reason: %v\n", r.Err)
		}
	}
	if r.Advice != "" {
		fmt.Fprintf(&b, "  %s\n", r.Advice)
	}
	return b.String()
}
