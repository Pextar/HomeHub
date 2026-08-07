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
