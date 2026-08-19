package api

import (
	"testing"

	"homehub/internal/announce"
	"homehub/internal/audio"
	"homehub/internal/autoplay"
	"homehub/internal/control"
	"homehub/internal/listening"
	"homehub/internal/media"
	"homehub/internal/music"
	"homehub/internal/musictimer"
	"homehub/internal/speakermon"
	"homehub/internal/store"
)

// newTestServer assembles a Server the way the composition root does, over a
// store the caller has already filled in.
//
// Every subsystem here is the real one, not a stand-in. None of them touches
// the network until something asks it to: the monitors neither poll nor
// subscribe before Run, the audio engine builds no decoder before a play, and
// autoplay ticks only under Run. So a test gets the actual wiring — the
// resolution a handler performs is the resolution under test — for the cost of
// a few struct literals.
//
// The monitors are built once and shared, because they are a *cache*: a music
// service reading one set of speaker states while the handlers read another
// would make the two disagree in exactly the way the single monitor exists to
// prevent.
func newTestServer(t *testing.T, st *store.Store) *Server {
	t.Helper()

	speakers := speakermon.New(speakermon.Config{
		Store:     st,
		HTTPPort:  "8080",
		EventPath: SonosEventPath,
	})
	engine := audio.New(audio.Config{
		StreamPath:  StreamPath,
		SpeakerAddr: st.AnySpeakerAddr,
		Quality:     func() media.StreamQuality { return testQuality(st) },
	})

	musicSvc := music.New(music.Config{
		Store:    st,
		Speakers: speakers,
		Audio:    engine,
	})

	return &Server{
		Store:       st,
		Control:     control.New(control.Config{Store: st}),
		SPADir:      t.TempDir(),
		Audio:       engine,
		Announce:    &announce.Service{PathPrefix: AnnouncePath},
		Speakers:    speakers,
		Music:       musicSvc,
		Autoplay:    autoplay.New(autoplay.Config{Store: st, Speakers: speakers}),
		MusicTimers: musictimer.New(musictimer.Config{Store: st, Music: musicSvc}),
		Listening: listening.New(listening.Config{
			Store:    st,
			Speakers: speakers,
			SonosArt: SonosArtURL,
			KEFArt:   KEFArtURL,
		}),
	}
}

// testQuality mirrors the composition root's reader, so a test that changes
// the household's setting sees the decoder follow it.
func testQuality(st *store.Store) media.StreamQuality {
	return media.StreamQuality(store.ViewValue(st, func() string {
		if st.Settings == nil {
			return ""
		}
		return st.Settings.StreamQuality
	}))
}

// testServer is newTestServer over an empty, loaded store — the starting point
// for a test that does not care what is in the house.
func testServer(t *testing.T) *Server {
	t.Helper()
	st := store.New(t.TempDir(), nil)
	if err := st.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	return newTestServer(t, st)
}
