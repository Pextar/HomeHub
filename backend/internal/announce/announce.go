// Package announce turns a sentence into audio the speakers can play, and
// serves it to them.
//
// The job is "call the kids for dinner from the wall panel": a chime so the
// room looks up, then the words. Two halves, and they fail independently.
//
//   - The chime is synthesised here (wav.go) and therefore always works. A
//     home with nothing configured still gets a doorbell in every room, which
//     is most of the value and needs no setup at all.
//   - The voice comes from a text-to-speech endpoint the household points
//     HomeHub at. There is no built-in speech: shipping a synthesiser is a
//     large dependency, and calling a cloud service uninvited would send the
//     household's sentences somewhere they didn't ask for. So it is opt-in,
//     it speaks the widely-implemented OpenAI /audio/speech shape (which
//     several local servers answer), and when it is absent or fails the
//     announcement degrades to the chime rather than failing.
//
// What this package does not do is decide *where* an announcement goes or how
// a room is put back afterwards — that is the bridge's job (see
// sonos.SnapshotTransport and friends), because only the bridge knows what a
// room was doing.
package announce

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// MaxTextLen caps what will be spoken. An announcement is a sentence; a
// paragraph read out at volume 40 in every room in the house is a mis-tap or
// a paste, and either way not something to hand to a synthesiser.
const MaxTextLen = 200

// leadIn is the pause between the chime and the first word.
const leadIn = 250 * time.Millisecond

// Dialect is the request shape a voice service speaks. Two, because the two
// realistic ways to give a house a voice speak different ones and neither is
// worth making the household run a translator for.
type Dialect string

const (
	// DialectOpenAI is POST {model, voice, input, response_format} — what
	// OpenAI's /v1/audio/speech takes and what most self-hosted servers
	// copy (Kokoro-FastAPI, Speaches, openedai-speech, LocalAI).
	DialectOpenAI Dialect = "openai"
	// DialectPiper is POST {text, voice} — Piper's own HTTP server, which
	// answers with a 16-bit PCM WAV directly. It is worth speaking natively
	// because Piper is the offline engine that has the languages a European
	// household actually needs, and because supporting it here saves that
	// household from running a second container purely to reshape a JSON
	// body.
	DialectPiper Dialect = "piper"
)

// Voice is a text-to-speech endpoint. Nil when the household hasn't
// configured one, which is a supported state and not an error.
type Voice struct {
	URL     string
	Dialect Dialect
	Model   string
	Name    string // the endpoint's voice id
	Key     string // optional bearer token
	HTTP    *http.Client
}

// VoiceFromEnv reads the optional TTS configuration.
//
// Environment rather than settings.json because it is deployment
// configuration — the address of another service on the household's own
// network — and it is exactly the kind of thing that should not be editable
// from a wall panel that anyone in the house can reach.
func VoiceFromEnv() *Voice {
	url := strings.TrimSpace(os.Getenv("HOMEHUB_TTS_URL"))
	if url == "" {
		return nil
	}
	v := &Voice{
		URL:     url,
		Dialect: DialectOpenAI,
		Model:   strings.TrimSpace(os.Getenv("HOMEHUB_TTS_MODEL")),
		Name:    strings.TrimSpace(os.Getenv("HOMEHUB_TTS_VOICE")),
		Key:     strings.TrimSpace(os.Getenv("HOMEHUB_TTS_KEY")),
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("HOMEHUB_TTS_KIND"))) {
	case string(DialectPiper):
		v.Dialect = DialectPiper
	case "", string(DialectOpenAI):
		// Piper's endpoint names itself, so the common case needs no
		// setting at all — a URL is enough to be pointed at either kind.
		// An explicit HOMEHUB_TTS_KIND still wins, for a proxy that has
		// moved the path.
		if strings.HasSuffix(strings.TrimRight(url, "/"), "/synthesize") {
			v.Dialect = DialectPiper
		}
	}
	if v.Dialect == DialectOpenAI {
		// Defaults that only mean anything to an OpenAI-shaped service.
		// Piper picks its voice from what the server was started with, and
		// sending it "alloy" would name a voice it has never heard of.
		if v.Model == "" {
			v.Model = "tts-1"
		}
		if v.Name == "" {
			v.Name = "alloy"
		}
	}
	return v
}

// requestBody is the only thing the two dialects disagree about. Both
// answer with a WAV, which is why everything after this point is shared.
func (v *Voice) requestBody(text string) map[string]any {
	if v.Dialect == DialectPiper {
		body := map[string]any{"text": text}
		// Optional: a Piper server started with one model needs no voice
		// named, and naming one it doesn't have would make it fall back
		// silently to its default.
		if v.Name != "" {
			body["voice"] = v.Name
		}
		return body
	}
	return map[string]any{
		"model": v.Model,
		"voice": v.Name,
		"input": text,
		// WAV explicitly: it is the one format that needs no decoder here,
		// and the chime has to be generated at its rate to be joined to it.
		"response_format": "wav",
	}
}

func (v *Voice) client() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// Speak renders text as 16-bit PCM WAV, in whichever dialect this endpoint
// speaks. Both answer with a WAV — the OpenAI shape because it is asked to,
// Piper because that is all it emits — so only the request differs.
func (v *Voice) Speak(ctx context.Context, text string) (Clip, error) {
	if v == nil || v.URL == "" {
		return Clip{}, ErrNoVoice
	}
	body, err := json.Marshal(v.requestBody(text))
	if err != nil {
		return Clip{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.URL, bytes.NewReader(body))
	if err != nil {
		return Clip{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if v.Key != "" {
		req.Header.Set("Authorization", "Bearer "+v.Key)
	}
	resp, err := v.client().Do(req)
	if err != nil {
		return Clip{}, fmt.Errorf("announce: reaching the voice service: %w", err)
	}
	defer resp.Body.Close()
	// 8 MB is about eight minutes of speech at 24 kHz mono — far past
	// MaxTextLen, and a bound on a service answering with something else.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Clip{}, fmt.Errorf("announce: reading the voice service: %w", err)
	}
	if resp.StatusCode >= 400 {
		return Clip{}, fmt.Errorf("announce: the voice service refused (%d): %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return parseWAV(raw)
}

// ErrNoVoice means no text-to-speech endpoint is configured. Callers treat
// it as "chime only", never as a failure.
var ErrNoVoice = fmt.Errorf("announce: no voice service is configured")

// Build composes the announcement: chime, then (when there is a voice and it
// answered) a pause and the words.
//
// The voice's format wins when there is one, and the chime is regenerated to
// match it — the alternative is resampling, which this package deliberately
// does not do (wav.go). A voice failure is reported alongside a usable
// chime-only clip rather than instead of it: the room still gets called.
func Build(ctx context.Context, v *Voice, text string) (Clip, error) {
	text = strings.TrimSpace(text)
	// By runes, not bytes: cutting mid-rune hands the synthesiser a broken
	// character, and every name with an å in it is a name someone will
	// actually announce.
	if r := []rune(text); len(r) > MaxTextLen {
		text = string(r[:MaxTextLen])
	}
	if text == "" || v == nil || v.URL == "" {
		return chime(defaultFormat), ErrNoVoice
	}
	speech, err := v.Speak(ctx, text)
	if err != nil {
		return chime(defaultFormat), err
	}
	return join(
		chime(speech.Format),
		Clip{Format: speech.Format, PCM: silence(speech.Format, leadIn)},
		speech,
	), nil
}

// Host serves built announcements over plain HTTP so speakers can fetch them.
//
// In memory and short-lived on purpose: an announcement is a few hundred
// kilobytes that is played once, within seconds, by speakers on the same LAN.
// Writing it to disk would mean a temp file to clean up on a device whose
// storage is a memory card.
type Host struct {
	// BaseURL is the address speakers should fetch from — the same
	// LAN-facing address the audio stream host uses, and it has the same
	// requirement: it must be reachable *by the speakers*.
	BaseURL string
	// PathPrefix is where Handler is mounted. Defaults to "/announce".
	PathPrefix string
	// TTL is how long a clip stays fetchable after it is published. It only
	// has to cover the speaker's own fetch, which happens within a second
	// of being told to play; the margin is for a speaker that has to wake
	// up first.
	TTL time.Duration

	mu    sync.Mutex
	clips map[string]hosted
}

type hosted struct {
	wav []byte
	at  time.Time
}

const defaultTTL = 2 * time.Minute

func (h *Host) prefix() string {
	if h.PathPrefix == "" {
		return "/announce"
	}
	return "/" + strings.Trim(h.PathPrefix, "/")
}

// Publish makes one clip fetchable and returns its URL.
func (h *Host) Publish(c Clip) (string, error) {
	if strings.TrimSpace(h.BaseURL) == "" {
		return "", fmt.Errorf("announce: no address the speakers can reach is configured for this server")
	}
	if len(c.PCM) == 0 {
		return "", fmt.Errorf("announce: nothing to play")
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf[:])

	h.mu.Lock()
	if h.clips == nil {
		h.clips = map[string]hosted{}
	}
	h.expire()
	h.clips[id] = hosted{wav: c.WAV(), at: time.Now()}
	h.mu.Unlock()

	// The .wav suffix is not decoration: some renderers pick their parser
	// from the URL before they look at the content type.
	return strings.TrimRight(h.BaseURL, "/") + h.prefix() + "/" + id + ".wav", nil
}

// expire drops clips past their TTL. Caller holds mu.
func (h *Host) expire() {
	ttl := h.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	for id, c := range h.clips {
		if time.Since(c.at) > ttl {
			delete(h.clips, id)
		}
	}
}

// Handler serves the published clips.
func (h *Host) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, h.prefix()+"/"), ".wav")
		h.mu.Lock()
		h.expire()
		c, ok := h.clips[id]
		h.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", fmt.Sprint(len(c.wav)))
		// Speakers range-request; ServeContent answers those correctly and
		// a plain Write does not.
		http.ServeContent(w, r, id+".wav", c.at, bytes.NewReader(c.wav))
	})
}
