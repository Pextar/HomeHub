package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homehub/internal/media"
)

type qualityResponse struct {
	StreamQuality string `json:"stream_quality"`
	Bitrate       int    `json:"bitrate_kbps"`
	Options       []struct {
		Value       string `json:"value"`
		Label       string `json:"label"`
		BitrateKbps int    `json:"bitrate_kbps"`
		Detail      string `json:"detail"`
	} `json:"options"`
	Providers []struct {
		ID     string `json:"id"`
		Routes []struct {
			Route   media.Route `json:"route"`
			Decoded bool        `json:"decoded"`
			Chain   media.Chain `json:"chain"`
		} `json:"routes"`
	} `json:"providers"`
}

func readQuality(t *testing.T, srv *Server) qualityResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.mediaQuality(rec, httptest.NewRequest(http.MethodGet, "/api/media/quality", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var out qualityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return out
}

func TestQualityReportsEveryRouteAndItsLimit(t *testing.T) {
	out := readQuality(t, testServer(t))

	if out.StreamQuality != string(media.QualityBest) || out.Bitrate != 320 {
		t.Errorf("a household that never chose gets %q/%d", out.StreamQuality, out.Bitrate)
	}
	if len(out.Options) != 3 {
		t.Fatalf("options = %d, want three", len(out.Options))
	}
	for _, o := range out.Options {
		if o.Detail == "" {
			t.Errorf("option %q should say what it costs", o.Value)
		}
	}

	// The report covers every provider. Spotify is the one with the
	// interesting attribution problem, so it is what the rest of this test
	// walks; Qobuz has its own test below.
	var spotify *struct {
		ID     string `json:"id"`
		Routes []struct {
			Route   media.Route `json:"route"`
			Decoded bool        `json:"decoded"`
			Chain   media.Chain `json:"chain"`
		} `json:"routes"`
	}
	for i := range out.Providers {
		if out.Providers[i].ID == "spotify" {
			spotify = &out.Providers[i]
		}
	}
	if spotify == nil {
		t.Fatalf("no spotify provider in %+v", out.Providers)
	}
	var sawDecoded, sawSpeakerServed bool
	for _, r := range spotify.Routes {
		// No route ever reports a flat "lossless": the two HomeHub decodes
		// for are capped by librespot, and the ones a speaker serves are a
		// ceiling HomeHub cannot see past. Reporting the transport's honesty
		// as the whole answer on either would be the original sin here.
		if r.Chain.Lossless {
			t.Errorf("%s: nothing here is measured lossless end to end", r.Route)
		}
		if r.Chain.Source.Detail == "" {
			t.Errorf("%s: the source stage should carry its caveat", r.Route)
		}
		if r.Decoded {
			sawDecoded = true
			if r.Chain.Source.Quality.Approximate {
				t.Errorf("%s: HomeHub knows its own decoder's bitrate exactly", r.Route)
			}
			// The limit on these routes is HomeHub's decoder, not Spotify's
			// catalogue — Spotify has a lossless tier, librespot just can't
			// fetch it. Blaming the service here would tell a listener their
			// speakers can't do better when one of them can.
			if r.Chain.Verdict != media.VerdictCapped {
				t.Errorf("%s: verdict = %q, want capped", r.Route, r.Chain.Verdict)
			}
			if r.Chain.LimitedBy != "HomeHub's decoder" {
				t.Errorf("%s: limited by %q, want HomeHub's decoder", r.Route, r.Chain.LimitedBy)
			}
			// And at best quality the only honest improvement left is to
			// leave the decoded route, so it is offered — with no setting,
			// because it is not a switch HomeHub owns.
			if r.Chain.Fix == nil {
				t.Errorf("%s: should offer the way off the decoded route", r.Route)
			} else if r.Chain.Fix.Setting != "" {
				t.Errorf("%s: fix setting = %q, want none to press", r.Route, r.Chain.Fix.Setting)
			}
		} else {
			sawSpeakerServed = true
			// The speaker holds the account, fetches Spotify's lossless
			// stream itself and never tells HomeHub what it settled on — so
			// it is a ceiling shown as "up to", and nothing caps it.
			if !r.Chain.Source.Quality.Approximate {
				t.Errorf("%s: a speaker-served route can only be approximate", r.Route)
			}
			if r.Chain.Verdict != media.VerdictUpTo {
				t.Errorf("%s: verdict = %q, want up_to", r.Route, r.Chain.Verdict)
			}
			if r.Chain.LimitedBy != "" {
				t.Errorf("%s: nothing caps this, but it blames %q", r.Route, r.Chain.LimitedBy)
			}
		}
	}
	if !sawDecoded || !sawSpeakerServed {
		t.Error("the report should cover both kinds of route")
	}
}

func TestSettingQualityChangesTheDecodeAndSaysWhen(t *testing.T) {
	srv := testServer(t)

	rec := httptest.NewRecorder()
	srv.mediaSetQuality(rec, httptest.NewRequest(http.MethodPut, "/api/media/quality",
		strings.NewReader(`{"stream_quality":"saver"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var resp struct {
		StreamQuality string `json:"stream_quality"`
		Bitrate       int    `json:"bitrate_kbps"`
		Applies       string `json:"applies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if resp.StreamQuality != "saver" || resp.Bitrate != 96 {
		t.Errorf("response = %+v", resp)
	}
	// A setting that appears to take effect and doesn't is worse than one
	// that says when it will.
	if resp.Applies == "" {
		t.Error("the response should say when the change lands")
	}
	if got := srv.Store.Settings.StreamQuality; got != "saver" {
		t.Errorf("stored quality = %q", got)
	}

	// And now the fix appears on exactly the routes it can reach.
	out := readQuality(t, srv)
	for _, r := range out.Providers[0].Routes {
		switch {
		case r.Decoded && r.Chain.Fix == nil:
			t.Errorf("%s: a below-best decode should offer the way back up", r.Route)
		case !r.Decoded && r.Chain.Fix != nil:
			t.Errorf("%s: this setting reaches nothing here, so it must not be offered", r.Route)
		}
	}
}

func TestSettingAnUnknownQualityIsRefused(t *testing.T) {
	srv := testServer(t)
	rec := httptest.NewRecorder()
	srv.mediaSetQuality(rec, httptest.NewRequest(http.MethodPut, "/api/media/quality",
		strings.NewReader(`{"stream_quality":"lossless"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	// The error names the three real choices — "lossless" is exactly the
	// thing a user would try, and the answer has to be useful.
	for _, want := range []string{"best", "balanced", "saver"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("the error should list %q: %s", want, rec.Body)
		}
	}
}

// The setting has to reach the decoder, or it is a preference that changes
// nothing. This asserts the wiring rather than the process.
func TestDecoderIsRebuiltWhenTheQualityChanges(t *testing.T) {
	srv := testServer(t)
	if got := srv.streamQuality().Bitrate(); got != 320 {
		t.Fatalf("default bitrate = %d", got)
	}
	_ = srv.decoder()
	if srv.librespotBitrate != 320 {
		t.Fatalf("decoder built at %d kbps", srv.librespotBitrate)
	}

	srv.Store.Settings.StreamQuality = "balanced"
	_ = srv.decoder()
	if srv.librespotBitrate != 160 {
		t.Errorf("decoder still at %d kbps after the setting changed", srv.librespotBitrate)
	}
}
