package announce

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A house with no text-to-speech configured is the common case, and it still
// has to be able to call the kids: the chime is synthesised here and always
// works. ErrNoVoice comes back beside a usable clip, never instead of one.
func TestBuildFallsBackToTheChime(t *testing.T) {
	clip, err := Build(context.Background(), nil, "Dinner's ready")
	if !errors.Is(err, ErrNoVoice) {
		t.Errorf("err = %v, want ErrNoVoice", err)
	}
	if len(clip.PCM) == 0 {
		t.Fatal("no audio was produced without a voice service")
	}
	if d := clip.Duration(); d < 400*time.Millisecond || d > 3*time.Second {
		t.Errorf("chime runs %v, want something between a beep and a doorbell", d)
	}
	if clip.Format != defaultFormat {
		t.Errorf("format = %+v, want the default", clip.Format)
	}
}

// The chime is regenerated at the *voice's* format so the two can be joined
// without a resampler. If that ever stops holding, the join silently drops
// the chime — so the test asserts the joined length, not just the format.
func TestBuildJoinsChimeAndSpeechAtTheVoiceFormat(t *testing.T) {
	speech := Clip{Format: Format{SampleRate: 24000, Channels: 1}}
	speech.PCM = make([]byte, 24000*2) // one second
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := decodeJSON(r, &body); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		if body["response_format"] != "wav" {
			t.Errorf("response_format = %v, want wav — nothing here can decode anything else", body["response_format"])
		}
		if body["input"] != "Dinner" {
			t.Errorf("input = %v, want the text", body["input"])
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(speech.WAV())
	}))
	defer srv.Close()

	clip, err := Build(context.Background(), &Voice{URL: srv.URL, HTTP: srv.Client()}, "Dinner")
	if err != nil {
		t.Fatal(err)
	}
	if clip.Format != speech.Format {
		t.Fatalf("format = %+v, want the voice's %+v", clip.Format, speech.Format)
	}
	chimeLen := chime(speech.Format).Duration()
	want := chimeLen + leadIn + time.Second
	if d := clip.Duration(); d < want-50*time.Millisecond || d > want+50*time.Millisecond {
		t.Errorf("clip runs %v, want ~%v (chime + lead-in + speech)", d, want)
	}
}

// A service that answers with something that isn't 16-bit PCM WAV is
// misconfigured. Guessing at the format is how a speaker ends up playing
// noise at volume 35, so the clip degrades to the chime instead.
func TestBuildRejectsAudioItCannotRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ID3\x04\x00not a wav at all"))
	}))
	defer srv.Close()

	clip, err := Build(context.Background(), &Voice{URL: srv.URL, HTTP: srv.Client()}, "Dinner")
	if !errors.Is(err, ErrUnsupportedAudio) {
		t.Errorf("err = %v, want ErrUnsupportedAudio", err)
	}
	if len(clip.PCM) == 0 {
		t.Error("the chime was lost along with the voice")
	}
}

func TestBuildReportsARefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unknown voice"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := Build(context.Background(), &Voice{URL: srv.URL, HTTP: srv.Client()}, "Dinner")
	if err == nil || !strings.Contains(err.Error(), "unknown voice") {
		t.Errorf("err = %v, want the service's own reason", err)
	}
}

// Real encoders put LIST and fact chunks before the audio. A parser that
// trusts the canonical 44-byte offset plays those bytes as sound.
func TestParseWAVWalksTheChunkList(t *testing.T) {
	base := Clip{Format: Format{SampleRate: 16000, Channels: 2}, PCM: []byte{1, 0, 2, 0, 3, 0, 4, 0}}
	wav := base.WAV()
	// Splice a LIST chunk between "fmt " and "data".
	list := []byte("LIST\x04\x00\x00\x00INFO")
	spliced := append([]byte{}, wav[:36]...)
	spliced = append(spliced, list...)
	spliced = append(spliced, wav[36:]...)
	binary.LittleEndian.PutUint32(spliced[4:], uint32(len(spliced)-8))

	got, err := parseWAV(spliced)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != base.Format || string(got.PCM) != string(base.PCM) {
		t.Errorf("parsed %+v, want %+v — the LIST chunk was read as audio", got, base)
	}
}

func TestParseWAVRejectsNonPCM(t *testing.T) {
	good := Clip{Format: Format{SampleRate: 16000, Channels: 1}, PCM: []byte{1, 0}}.WAV()
	mp3ish := append([]byte{}, good...)
	binary.LittleEndian.PutUint16(mp3ish[20:], 3) // IEEE float, not PCM
	if _, err := parseWAV(mp3ish); !errors.Is(err, ErrUnsupportedAudio) {
		t.Errorf("err = %v, want ErrUnsupportedAudio", err)
	}
	if _, err := parseWAV([]byte("nope")); !errors.Is(err, ErrUnsupportedAudio) {
		t.Errorf("err = %v, want ErrUnsupportedAudio", err)
	}
}

// The WAV a speaker fetches has to round-trip through our own parser, which
// is the closest thing to a renderer this test can hold.
func TestWAVRoundTrips(t *testing.T) {
	want := chime(Format{SampleRate: 22050, Channels: 2})
	got, err := parseWAV(want.WAV())
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != want.Format || len(got.PCM) != len(want.PCM) {
		t.Errorf("round trip = %+v/%d bytes, want %+v/%d", got.Format, len(got.PCM), want.Format, len(want.PCM))
	}
}

// Clips that don't share a format are dropped rather than mangled: appending
// 24 kHz mono to 44.1 kHz stereo produces something that plays as noise.
func TestJoinRefusesMismatchedFormats(t *testing.T) {
	a := Clip{Format: Format{SampleRate: 44100, Channels: 1}, PCM: []byte{1, 0, 2, 0}}
	b := Clip{Format: Format{SampleRate: 24000, Channels: 1}, PCM: []byte{3, 0}}
	got := join(a, b)
	if got.Format != a.Format || len(got.PCM) != len(a.PCM) {
		t.Errorf("join = %+v/%d bytes, want the first clip untouched", got.Format, len(got.PCM))
	}
}

// The clip is fetched by a speaker, not by a browser: it needs a real
// content type, and it stops existing shortly afterwards.
func TestHostServesAndExpires(t *testing.T) {
	h := &Host{BaseURL: "http://192.168.1.10:8080", TTL: time.Hour}
	url, err := h.Publish(chime(defaultFormat))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://192.168.1.10:8080/announce/") || !strings.HasSuffix(url, ".wav") {
		t.Errorf("url = %q, want a .wav under the announce prefix", url)
	}

	path := strings.TrimPrefix(url, "http://192.168.1.10:8080")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET clip = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/wav" {
		t.Errorf("content type = %q, want audio/wav", ct)
	}
	if _, err := parseWAV(rec.Body.Bytes()); err != nil {
		t.Errorf("served body is not a readable WAV: %v", err)
	}

	// Past its TTL it is gone, and an unknown id was never there.
	h.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	rec = httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expired clip = %d, want 404", rec.Code)
	}
}

func TestHostNeedsAReachableAddress(t *testing.T) {
	h := &Host{}
	if _, err := h.Publish(chime(defaultFormat)); err == nil {
		t.Error("Publish with no BaseURL = nil, want an error naming the missing address")
	}
}

// Build truncates rather than handing a paragraph to a synthesiser to read
// out at volume 35 in every room in the house.
func TestBuildCapsTheText(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = decodeJSON(r, &body)
		got, _ = body["input"].(string)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(Clip{Format: Format{SampleRate: 16000, Channels: 1}, PCM: []byte{0, 0}}.WAV())
	}))
	defer srv.Close()

	long := strings.Repeat("a", MaxTextLen+50)
	if _, err := Build(context.Background(), &Voice{URL: srv.URL, HTTP: srv.Client()}, long); err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxTextLen {
		t.Errorf("spoke %d characters, want them capped at %d", len(got), MaxTextLen)
	}
}

// decodeJSON is a test helper: the request bodies here are small and read
// once, so this stays local rather than becoming package surface.
func decodeJSON(r *http.Request, out any) error {
	return json.NewDecoder(r.Body).Decode(out)
}

// A WAV written to an HTTP response cannot know its own length, and every
// convention for saying so has to read as "the rest of what arrived". This
// is the normal case for a TTS endpoint that streams its answer, not a
// corner case — and getting it wrong drops the voice and leaves the house
// wondering why the panel only chimes.
func TestParseWAVAcceptsStreamedLengths(t *testing.T) {
	real := Clip{Format: Format{SampleRate: 24000, Channels: 1}, PCM: []byte{1, 0, 2, 0, 3, 0, 4, 0}}
	for _, declared := range []uint32{0, 0xFFFFFFFF, 0xFFFFFF7F, 9999} {
		wav := real.WAV()
		binary.LittleEndian.PutUint32(wav[40:], declared) // the data chunk's size
		got, err := parseWAV(wav)
		if err != nil {
			t.Errorf("data size %#x: %v", declared, err)
			continue
		}
		if string(got.PCM) != string(real.PCM) || got.Format != real.Format {
			t.Errorf("data size %#x gave %d bytes at %+v, want %d at %+v",
				declared, len(got.PCM), got.Format, len(real.PCM), real.Format)
		}
	}
}

// ...but a zero-length chunk that isn't the audio is legitimate, and must
// not swallow the data chunk sitting behind it.
func TestParseWAVDoesNotLetAnEmptyChunkEatTheAudio(t *testing.T) {
	real := Clip{Format: Format{SampleRate: 16000, Channels: 1}, PCM: []byte{5, 0, 6, 0}}
	wav := real.WAV()
	empty := []byte("LIST\x00\x00\x00\x00")
	spliced := append([]byte{}, wav[:36]...)
	spliced = append(spliced, empty...)
	spliced = append(spliced, wav[36:]...)
	binary.LittleEndian.PutUint32(spliced[4:], uint32(len(spliced)-8))

	got, err := parseWAV(spliced)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.PCM) != string(real.PCM) {
		t.Errorf("PCM = %v, want %v — an empty LIST chunk ate the audio", got.PCM, real.PCM)
	}
}

// The two dialects differ in exactly one place — the request body — and the
// wrong one is a silent failure: Piper ignores fields it doesn't know and
// synthesises an empty string, so the house gets a chime and no explanation.
func TestPiperDialectSendsPipersOwnBody(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &got)
		w.Header().Set("Content-Type", "audio/wav")
		// Piper answers at its voice's own rate, mono — which is why the
		// chime is regenerated to match rather than assumed.
		_, _ = w.Write(Clip{Format: Format{SampleRate: 22050, Channels: 1}, PCM: make([]byte, 4410)}.WAV())
	}))
	defer srv.Close()

	v := &Voice{URL: srv.URL + "/synthesize", Dialect: DialectPiper, Name: "sv_SE-alma-medium", HTTP: srv.Client()}
	clip, err := Build(context.Background(), v, "Maten är klar")
	if err != nil {
		t.Fatal(err)
	}
	if got["text"] != "Maten är klar" {
		t.Errorf("body = %v, want Piper's own {text: …}", got)
	}
	if got["voice"] != "sv_SE-alma-medium" {
		t.Errorf("voice = %v, want the configured one", got["voice"])
	}
	if _, ok := got["input"]; ok {
		t.Error("the OpenAI field 'input' was sent to a Piper server")
	}
	if clip.Format.SampleRate != 22050 {
		t.Errorf("clip rate = %d, want the voice's 22050", clip.Format.SampleRate)
	}
}

// A Piper server names itself in its own URL, so pointing HomeHub at one is
// a single setting. An explicit kind still wins, for a proxy that moved the
// path.
func TestVoiceFromEnvPicksTheDialect(t *testing.T) {
	cases := []struct {
		url, kind string
		want      Dialect
	}{
		{"http://pi.local:5000/synthesize", "", DialectPiper},
		{"http://pi.local:5000/synthesize/", "", DialectPiper},
		{"http://pi.local:8880/v1/audio/speech", "", DialectOpenAI},
		{"http://pi.local:8880/tts", "piper", DialectPiper},
		{"http://pi.local:5000/synthesize", "openai", DialectPiper}, // suffix still wins
	}
	for _, c := range cases {
		t.Setenv("HOMEHUB_TTS_URL", c.url)
		t.Setenv("HOMEHUB_TTS_KIND", c.kind)
		v := VoiceFromEnv()
		if v == nil {
			t.Fatalf("%s: no voice configured", c.url)
		}
		if v.Dialect != c.want {
			t.Errorf("%s (kind %q) = %s, want %s", c.url, c.kind, v.Dialect, c.want)
		}
	}

	// And an OpenAI-shaped service gets the defaults a Piper one must not:
	// naming "alloy" at a Piper server picks a voice it has never heard of.
	t.Setenv("HOMEHUB_TTS_URL", "http://pi.local:5000/synthesize")
	t.Setenv("HOMEHUB_TTS_KIND", "")
	if v := VoiceFromEnv(); v.Name != "" || v.Model != "" {
		t.Errorf("piper voice defaulted to model=%q voice=%q, want neither", v.Model, v.Name)
	}
	t.Setenv("HOMEHUB_TTS_URL", "http://pi.local:8880/v1/audio/speech")
	if v := VoiceFromEnv(); v.Name != "alloy" || v.Model != "tts-1" {
		t.Errorf("openai voice = model=%q voice=%q, want the defaults", v.Model, v.Name)
	}
}

func TestNoVoiceWithoutAURL(t *testing.T) {
	t.Setenv("HOMEHUB_TTS_URL", "")
	if VoiceFromEnv() != nil {
		t.Error("a voice was configured out of nothing")
	}
}
