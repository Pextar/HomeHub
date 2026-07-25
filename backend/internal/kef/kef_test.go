package kef

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSpeaker stands in for a KEF speaker: it holds a path→envelope map and
// serves the two calls the real device does. Tests address it by a plausible
// LAN IP (ValidateHost rejects loopback on purpose), so the package's own
// HTTP client is redirected at the test server for the duration.
type fakeSpeaker struct {
	t *testing.T

	mu     sync.Mutex
	values map[string]string // path → raw envelope JSON
	// controls records the transport actions sent, in order.
	controls []string
	// writes records path→raw value envelope for every setData.
	writes map[string]string
	// fail is a set of paths that answer HTTP 500, standing in for a model
	// that doesn't have them.
	fail map[string]bool
	// hits counts requests per path.
	hits map[string]int
}

const testIP = "192.168.1.60"

func newFakeSpeaker(t *testing.T) *fakeSpeaker {
	t.Helper()
	f := &fakeSpeaker{
		t:      t,
		values: map[string]string{},
		writes: map[string]string{},
		fail:   map[string]bool{},
		hits:   map[string]int{},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	prev := client
	client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}
	t.Cleanup(func() { client = prev })
	return f
}

func (f *fakeSpeaker) set(path, envelope string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[path] = envelope
}

func (f *fakeSpeaker) wrote(path string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.writes[path]
	return v, ok
}

func (f *fakeSpeaker) hitCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fakeSpeaker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/getData":
		path := r.URL.Query().Get("path")
		f.mu.Lock()
		f.hits[path]++
		envelope, ok := f.values[path]
		bad := f.fail[path]
		f.mu.Unlock()
		if bad || !ok {
			http.Error(w, "no such path", http.StatusInternalServerError)
			return
		}
		if got := r.URL.Query().Get("roles"); got != "value" {
			f.t.Errorf("getData roles = %q, want value", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "["+envelope+"]")

	case "/api/setData":
		var body struct {
			Path  string          `json:"path"`
			Role  string          `json:"role"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.hits[body.Path]++
		bad := f.fail[body.Path]
		if !bad {
			f.writes[body.Path] = string(body.Value)
			if body.Path == pathControl {
				var ctl struct {
					Control string `json:"control"`
				}
				_ = json.Unmarshal(body.Value, &ctl)
				f.controls = append(f.controls, ctl.Control)
			} else {
				// A write is visible to the next read, like the real thing.
				f.values[body.Path] = string(body.Value)
			}
		}
		f.mu.Unlock()
		if bad {
			http.Error(w, "refused", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.NotFound(w, r)
	}
}

// describable loads the paths every speaker answers for.
func (f *fakeSpeaker) describable() {
	f.set(pathDeviceName, `{"type":"string_","string_":"Kitchen"}`)
	f.set(pathMAC, `{"type":"string_","string_":"A1:B2:C3:D4:E5:F6"}`)
	f.set(pathModel, `{"type":"string_","string_":"LS50 Wireless II"}`)
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

// ── Host validation ──────────────────────────────────────────────────────

func TestValidateHost(t *testing.T) {
	ok := []string{"192.168.1.60", "10.0.0.5", "kef-kitchen", "speaker.local", "172.16.4.4"}
	for _, h := range ok {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", h, err)
		}
	}
	// Rejected for the same reasons as the Sonos bridge: anything that could
	// redirect a server-side request, and addresses that aren't a LAN device.
	bad := []string{
		"", "  ", "192.168.1.60:80", "192.168.1.60/api", "evil.com/../x",
		"user@192.168.1.60", "127.0.0.1", "::1", "0.0.0.0", "169.254.1.1",
		"224.0.0.1", "host name", "host\nname",
	}
	for _, h := range bad {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want an error", h)
		}
	}
}

// ── Value envelope ───────────────────────────────────────────────────────

func TestValueRoundTrip(t *testing.T) {
	cases := []struct {
		typ string
		in  any
	}{
		{typeI32, 45},
		{typeBool, true},
		{typeString, "Living Room"},
		{typeSource, SourceOptical},
	}
	for _, c := range cases {
		v, err := newValue(c.typ, c.in)
		if err != nil {
			t.Fatalf("newValue(%s): %v", c.typ, err)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal(%s): %v", c.typ, err)
		}
		var back value
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("unmarshal(%s) of %s: %v", c.typ, raw, err)
		}
		if back.Type != c.typ {
			t.Errorf("round trip type = %q, want %q", back.Type, c.typ)
		}
		if string(back.Raw) == "" {
			t.Errorf("round trip of %s lost its payload", c.typ)
		}
	}
}

func TestValueDecodeRejectsWrongType(t *testing.T) {
	var v value
	if err := json.Unmarshal([]byte(`{"type":"string_","string_":"45"}`), &v); err != nil {
		t.Fatal(err)
	}
	var n int
	// A string where an int was expected means we read the wrong path;
	// returning 0 would put a wrong number on screen.
	if err := v.decode(typeI32, &n); err == nil {
		t.Error("decode accepted a string_ payload as i32_")
	}
}

func TestGetValueAcceptsBareObject(t *testing.T) {
	// Older firmware answers with the object rather than a one-element array.
	f := newFakeSpeaker(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"type":"i32_","i32_":31}`)
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	client = &http.Client{Transport: &http.Transport{
		DialContext: func(c context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(c, network, addr)
		},
	}}
	_ = f

	got, err := getInt(ctx(t), testIP, pathVolume)
	if err != nil {
		t.Fatalf("getInt: %v", err)
	}
	if got != 31 {
		t.Errorf("getInt = %d, want 31", got)
	}
}

// ── Identity ─────────────────────────────────────────────────────────────

func TestDescribe(t *testing.T) {
	f := newFakeSpeaker(t)
	f.describable()

	d, err := Describe(ctx(t), testIP)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if d.Name != "Kitchen" {
		t.Errorf("Name = %q, want Kitchen", d.Name)
	}
	if d.MAC != "a1b2c3d4e5f6" {
		t.Errorf("MAC = %q, want a1b2c3d4e5f6", d.MAC)
	}
	if d.Model != "LS50 Wireless II" {
		t.Errorf("Model = %q", d.Model)
	}
	if d.IP != testIP {
		t.Errorf("IP = %q, want %q", d.IP, testIP)
	}
}

func TestDescribeFallsBackToReleaseText(t *testing.T) {
	// Firmware without modelName still identifies itself in releasetext.
	f := newFakeSpeaker(t)
	f.describable()
	f.mu.Lock()
	delete(f.values, pathModel)
	f.mu.Unlock()
	f.set(pathRelease, `{"type":"string_","string_":"LS50W2_2.2.1"}`)

	d, err := Describe(ctx(t), testIP)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if d.Model != "LS50 Wireless II" {
		t.Errorf("Model = %q, want the name mapped from the build code", d.Model)
	}
}

func TestDescribeRejectsNonKEF(t *testing.T) {
	// Something answering HTTP on :80 that isn't a speaker must not register.
	newFakeSpeaker(t)
	if _, err := Describe(ctx(t), testIP); err == nil {
		t.Fatal("Describe accepted a device that answered nothing")
	}
}

func TestNormalizeMAC(t *testing.T) {
	cases := map[string]string{
		"A1:B2:C3:D4:E5:F6": "a1b2c3d4e5f6",
		"a1-b2-c3-d4-e5-f6": "a1b2c3d4e5f6",
		" a1b2c3d4e5f6 ":    "a1b2c3d4e5f6",
		"a1:b2:c3:d4:e5":    "", // too short to be an id
		"":                  "",
		"not-a-mac":         "",
	}
	for in, want := range cases {
		if got := NormalizeMAC(in); got != want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModelFromCode(t *testing.T) {
	if got := ModelFromCode("ls50w2"); got != "LS50 Wireless II" {
		t.Errorf("ModelFromCode(ls50w2) = %q", got)
	}
	// An unknown code is passed through — an unfamiliar name beats a wrong one.
	if got := ModelFromCode("LSFUTURE"); got != "LSFUTURE" {
		t.Errorf("ModelFromCode of an unknown code = %q, want it unchanged", got)
	}
	if got := ModelFromCode(""); got != "" {
		t.Errorf("ModelFromCode(\"\") = %q", got)
	}
}

// ── Controls ─────────────────────────────────────────────────────────────

func TestTransportControls(t *testing.T) {
	f := newFakeSpeaker(t)
	for _, c := range []struct {
		fn   func(context.Context, string) error
		want string
	}{
		{Play, ControlPlay},
		{Pause, ControlPause},
		{Next, ControlNext},
		{Previous, ControlPrevious},
		{Stop, ControlStop},
	} {
		if err := c.fn(ctx(t), testIP); err != nil {
			t.Fatalf("%s: %v", c.want, err)
		}
	}
	f.mu.Lock()
	got := strings.Join(f.controls, ",")
	f.mu.Unlock()
	want := "play,pause,next,previous,stop"
	if got != want {
		t.Errorf("controls sent = %q, want %q", got, want)
	}
}

func TestSetVolumeClamps(t *testing.T) {
	f := newFakeSpeaker(t)
	if err := SetVolume(ctx(t), testIP, 140); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	got, _ := f.wrote(pathVolume)
	if !strings.Contains(got, `"i32_":100`) {
		t.Errorf("SetVolume(140) wrote %s, want it clamped to 100", got)
	}
	if err := SetVolume(ctx(t), testIP, -5); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	got, _ = f.wrote(pathVolume)
	if !strings.Contains(got, `"i32_":0`) {
		t.Errorf("SetVolume(-5) wrote %s, want it clamped to 0", got)
	}
}

func TestSetSourceRejectsUnknown(t *testing.T) {
	newFakeSpeaker(t)
	if err := SetSource(ctx(t), testIP, "aux7"); err == nil {
		t.Error("SetSource accepted a source the speaker has no name for")
	}
	if err := SetSource(ctx(t), testIP, SourceOptical); err != nil {
		t.Errorf("SetSource(optic): %v", err)
	}
}

func TestSetStandbyWakesViaSource(t *testing.T) {
	// Waking is a source selection: the speaker ignores a write of powerOn.
	f := newFakeSpeaker(t)
	if err := SetStandby(ctx(t), testIP, false); err != nil {
		t.Fatalf("SetStandby(false): %v", err)
	}
	if got, ok := f.wrote(pathSource); !ok || !strings.Contains(got, SourceWiFi) {
		t.Errorf("waking wrote %q to the source path, want wifi", got)
	}
	if _, ok := f.wrote(pathSpeakerStatus); ok {
		t.Error("waking wrote to speakerStatus, which the speaker ignores")
	}

	if err := SetStandby(ctx(t), testIP, true); err != nil {
		t.Fatalf("SetStandby(true): %v", err)
	}
	if got, ok := f.wrote(pathSpeakerStatus); !ok || !strings.Contains(got, StatusStandby) {
		t.Errorf("standby wrote %q, want standby", got)
	}
}

// ── Now playing ──────────────────────────────────────────────────────────

const playerDataJSON = `{
  "state": "playing",
  "status": {"name": "playing"},
  "trackRoles": {
    "title": "Teardrop",
    "icon": "https://art.example/teardrop.jpg",
    "mediaData": {
      "metaData": {"artist": "Massive Attack", "album": "Mezzanine", "duration": 330000}
    }
  }
}`

func TestParsePlayerData(t *testing.T) {
	track, status, dur := ParsePlayerData([]byte(playerDataJSON))
	if track == nil {
		t.Fatal("ParsePlayerData returned no track")
	}
	if track.Title != "Teardrop" || track.Artist != "Massive Attack" || track.Album != "Mezzanine" {
		t.Errorf("track = %+v", track)
	}
	if track.ArtURI != "https://art.example/teardrop.jpg" {
		t.Errorf("ArtURI = %q", track.ArtURI)
	}
	if status != StatusPlaying {
		t.Errorf("status = %q, want playing", status)
	}
	if dur != 330000 {
		t.Errorf("duration = %d, want 330000", dur)
	}
}

func TestParsePlayerDataEmpty(t *testing.T) {
	// A speaker on line-in reports a player with no metadata; that is a
	// valid answer, not a parse failure.
	track, status, dur := ParsePlayerData([]byte(`{"status":{"name":"stopped"},"trackRoles":{}}`))
	if track != nil {
		t.Errorf("track = %+v, want nil", track)
	}
	if status != StatusStopped {
		t.Errorf("status = %q, want stopped", status)
	}
	if dur != 0 {
		t.Errorf("duration = %d, want 0", dur)
	}
	if track, _, _ := ParsePlayerData([]byte(`not json`)); track != nil {
		t.Error("ParsePlayerData returned a track for junk input")
	}
}

func TestParsePlayerDataFallsBackToState(t *testing.T) {
	track, status, dur := ParsePlayerData([]byte(`{
	  "state":"paused",
	  "trackRoles":{"title":"B","mediaData":{"resources":[{"duration":1000}]}}}`))
	if status != StatusPaused {
		t.Errorf("status = %q, want paused from the state field", status)
	}
	if dur != 1000 {
		t.Errorf("duration = %d, want it from resources", dur)
	}
	if track == nil || track.Title != "B" {
		t.Errorf("track = %+v", track)
	}
}

func TestGetState(t *testing.T) {
	f := newFakeSpeaker(t)
	f.set(pathSpeakerStatus, `{"type":"kefSpeakerStatus","kefSpeakerStatus":"powerOn"}`)
	f.set(pathSource, `{"type":"kefPhysicalSource","kefPhysicalSource":"wifi"}`)
	f.set(pathVolume, `{"type":"i32_","i32_":34}`)
	f.set(pathMute, `{"type":"bool_","bool_":false}`)
	f.set(pathPlayerData, `{"type":"playerData","playerData":`+playerDataJSON+`}`)
	f.set(pathPlayTime, `{"type":"i64_","i64_":42000}`)

	st, err := GetState(ctx(t), testIP)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if !st.PoweredOn || !st.Playing || st.Status != StatusPlaying {
		t.Errorf("state = %+v, want an awake, playing speaker", st)
	}
	if st.Volume != 34 || st.Muted {
		t.Errorf("volume/mute = %d/%v", st.Volume, st.Muted)
	}
	if st.Source != SourceWiFi {
		t.Errorf("source = %q", st.Source)
	}
	if st.Track == nil || st.Track.Title != "Teardrop" {
		t.Errorf("track = %+v", st.Track)
	}
	if st.PositionMS != 42000 || st.DurationMS != 330000 {
		t.Errorf("position/duration = %d/%d", st.PositionMS, st.DurationMS)
	}
}

func TestGetStateStandby(t *testing.T) {
	// A speaker in standby answers, and the answer is worth rendering: the
	// transport doing nothing has a reason, and it isn't "unreachable".
	f := newFakeSpeaker(t)
	f.set(pathSpeakerStatus, `{"type":"kefSpeakerStatus","kefSpeakerStatus":"standby"}`)
	f.set(pathSource, `{"type":"kefPhysicalSource","kefPhysicalSource":"standby"}`)

	st, err := GetState(ctx(t), testIP)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.PoweredOn {
		t.Error("PoweredOn = true for a speaker in standby")
	}
	if st.Playing || st.Status != StatusStopped {
		t.Errorf("state = %+v, want stopped", st)
	}
	if st.Track != nil {
		t.Errorf("track = %+v, want nil", st.Track)
	}
}

func TestGetStateUnreachable(t *testing.T) {
	// Nothing answers the power path — the whole read fails rather than
	// reporting a speaker that is awake and silent.
	newFakeSpeaker(t)
	if _, err := GetState(ctx(t), testIP); err == nil {
		t.Fatal("GetState succeeded against a speaker that answered nothing")
	}
}

// ── Settings ─────────────────────────────────────────────────────────────

func TestGetSettingsOmitsPathsTheModelLacks(t *testing.T) {
	f := newFakeSpeaker(t)
	f.describable()
	f.set(pathBassExtension, `{"type":"string_","string_":"extra"}`)
	f.set(pathDeskMode, `{"type":"bool_","bool_":true}`)
	f.set(pathDeskModeSet, `{"type":"i32_","i32_":-25}`)
	f.set(pathTrebleAmount, `{"type":"i32_","i32_":5}`)
	f.set(pathStandbyMode, `{"type":"kefStandbyMode","kefStandbyMode":"standby_20mins"}`)
	f.set(pathMaxVolume, `{"type":"i32_","i32_":80}`)
	// No subwoofer paths at all — an LSX II has no sub output.

	s, err := GetSettings(ctx(t), testIP)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.BassExtension == nil || *s.BassExtension != BassExtra {
		t.Errorf("bass extension = %v", s.BassExtension)
	}
	if s.DeskMode == nil || !*s.DeskMode {
		t.Errorf("desk mode = %v", s.DeskMode)
	}
	if s.DeskGain == nil || *s.DeskGain != -25 {
		t.Errorf("desk gain = %v", s.DeskGain)
	}
	if s.StandbyMode == nil || *s.StandbyMode != Standby20Min {
		t.Errorf("standby mode = %v", s.StandbyMode)
	}
	// The point of the pointers: absent is not false.
	if s.SubwooferOut != nil {
		t.Errorf("subwoofer_out = %v, want nil for a model without one", s.SubwooferOut)
	}
	if s.SubGain != nil || s.SubLPFreq != nil || s.HighPassMode != nil {
		t.Error("a model without a sub output reported sub settings")
	}
	if s.Info.Model != "LS50 Wireless II" || s.Info.MAC != "a1b2c3d4e5f6" {
		t.Errorf("info = %+v", s.Info)
	}
}

func TestGetSettingsRequiresAReachableSpeaker(t *testing.T) {
	newFakeSpeaker(t)
	if _, err := GetSettings(ctx(t), testIP); err == nil {
		t.Fatal("GetSettings succeeded against an unreachable speaker")
	}
}

func TestSettingsPatchValidate(t *testing.T) {
	s := func(v string) *string { return &v }
	n := func(v int) *int { return &v }

	bad := []SettingsPatch{
		{BassExtension: s("massive")},
		{SubPhase: s("phase90")},
		{StandbyMode: s("standby_5mins")},
		{DeskGain: n(10)},     // above 0 dB
		{WallGain: n(-100)},   // below -6 dB
		{Treble: n(50)},       // beyond ±2 dB
		{HighPassFreq: n(30)}, // below 50 Hz
		{SubLPFreq: n(400)},   // above 250 Hz
		{SubGain: n(-20)},     // beyond ±10 dB
		{MaxVolume: n(101)},   // beyond the volume scale
	}
	for _, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("Validate accepted %+v", p)
		}
	}

	good := SettingsPatch{
		BassExtension: s(BassStandard),
		SubPhase:      s(Phase180),
		StandbyMode:   s(Standby60Min),
		DeskGain:      n(-60),
		WallGain:      n(0),
		Treble:        n(-20),
		HighPassFreq:  n(120),
		SubLPFreq:     n(40),
		SubGain:       n(10),
		MaxVolume:     n(100),
	}
	if err := good.Validate(); err != nil {
		t.Errorf("Validate rejected a patch at every boundary: %v", err)
	}
	if !(SettingsPatch{}).Empty() {
		t.Error("an empty patch didn't report itself empty")
	}
	if good.Empty() {
		t.Error("a populated patch reported itself empty")
	}
}

func TestApplySettingsWritesOnlyWhatItWasGiven(t *testing.T) {
	f := newFakeSpeaker(t)
	on := true
	gain := -30
	if err := ApplySettings(ctx(t), testIP, SettingsPatch{DeskMode: &on, DeskGain: &gain}); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if got, ok := f.wrote(pathDeskMode); !ok || !strings.Contains(got, "true") {
		t.Errorf("desk mode wrote %q", got)
	}
	if got, ok := f.wrote(pathDeskModeSet); !ok || !strings.Contains(got, "-30") {
		t.Errorf("desk gain wrote %q", got)
	}
	// One flipped switch must not cost a write to every other path.
	if _, ok := f.wrote(pathWallMode); ok {
		t.Error("ApplySettings wrote a field the patch didn't carry")
	}
	if f.hitCount(pathTrebleAmount) != 0 {
		t.Error("ApplySettings touched treble without being asked to")
	}
}

func TestApplySettingsReportsARefusal(t *testing.T) {
	f := newFakeSpeaker(t)
	f.mu.Lock()
	f.fail[pathPhaseCorrect] = true
	f.mu.Unlock()

	on := true
	err := ApplySettings(ctx(t), testIP, SettingsPatch{PhaseCorrect: &on})
	if err == nil {
		t.Fatal("ApplySettings swallowed a refusal from the speaker")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want it to name the speaker's refusal", err)
	}
}

func TestApplySettingsValidatesBeforeSending(t *testing.T) {
	f := newFakeSpeaker(t)
	bad := 999
	if err := ApplySettings(ctx(t), testIP, SettingsPatch{SubGain: &bad}); err == nil {
		t.Fatal("ApplySettings sent an out-of-range value")
	}
	if f.hitCount(pathSubwooferGain) != 0 {
		t.Error("ApplySettings reached the speaker with a value it would refuse")
	}
}
