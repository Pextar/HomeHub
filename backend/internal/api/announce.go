package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"homehub/internal/announce"
	"homehub/internal/sonos"
	"homehub/internal/store"
)

// Calling the house: one sentence, every room by default, and the music put
// back.
//
// This is the panel's feature more than the app's — "dinner's ready" is
// shouted from a hallway, not typed on a phone — and the shape follows from
// that. Left unaddressed it goes to every reachable Sonos coordinator at
// once, because a household announcement that reaches one room is a worse
// answer than none — but a caller may name specific rooms to narrow it, for
// the times only one room needs calling. Followers are not addressed: a
// grouped speaker plays what its coordinator plays, so announcing to both
// would double the audio in one room.
//
// KEF speakers are deliberately not included. Their own API cannot report
// what they are playing, so there is nothing to snapshot and a clip would end
// with the room silent and the music gone. §15.1's rule holds here as
// everywhere: a control that would be refused — or that would quietly cost
// something — is worse than a control that isn't there. The response says how
// many rooms will hear it so the panel can be honest about the reach.

const (
	// announceVolume is how loud an announcement plays, as a group volume.
	// High enough to carry over a kitchen, low enough not to be alarming in
	// a room where someone is sitting next to the speaker.
	announceVolume = 35

	// announceTail is the margin after the clip's own length before the
	// room is handed back. It covers the speaker's buffering and the fade
	// at the end; without it the last word is cut by the restore.
	announceTail = 1200 * time.Millisecond

	// announceRestoreBudget bounds the background restore. It is generous:
	// the restore is several SOAP calls per room and it is the half that
	// must not be abandoned.
	announceRestoreBudget = 45 * time.Second
)

// announcePath is where the clips are served. Outside /api for the same
// reason the stream is: the clients are speakers, and /api is session-gated.
const announcePath = "/announce"

// announceHost returns the clip host, creating it on first use. It shares the
// stream host's address discovery — the requirement is identical (an address
// the *speakers* can reach) and solving it twice would mean two ways to be
// wrong on a multi-homed box.
func (s *Server) announceHost() *announce.Host {
	s.announceMu.Lock()
	defer s.announceMu.Unlock()
	if s.announcer != nil {
		return s.announcer
	}
	base := s.streamBaseURL()
	if base == "" {
		return nil
	}
	s.announcer = &announce.Host{BaseURL: base, PathPrefix: announcePath}
	return s.announcer
}

// announceHandler serves the published clips to speakers.
func (s *Server) announceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.announceMu.Lock()
		host := s.announcer
		s.announceMu.Unlock()
		if host == nil {
			http.NotFound(w, r)
			return
		}
		host.Handler().ServeHTTP(w, r)
	})
}

// announceBegin claims the household for one announcement, reporting false
// when another is still playing. announceEnd releases it.
func (s *Server) announceBegin() bool {
	s.announceMu.Lock()
	defer s.announceMu.Unlock()
	if s.announcing {
		return false
	}
	s.announcing = true
	return true
}

func (s *Server) announceEnd() {
	s.announceMu.Lock()
	s.announcing = false
	s.announceMu.Unlock()
}

// announceStatus handles GET /api/announce — what the surface needs to draw
// the control before anyone taps it: whether there is anywhere to announce
// to, and whether it will be spoken or only chimed.
func (s *Server) announceStatus(w http.ResponseWriter, r *http.Request) {
	rooms := s.announceTargets()
	// id alongside name: the panel needs something stable to select by, and
	// a display name isn't it — two rooms can share one.
	list := make([]map[string]string, 0, len(rooms))
	for _, t := range rooms {
		list = append(list, map[string]string{"id": t.ID, "name": t.Name})
	}
	// The sentences the panel offers before its box. They come from
	// household settings rather than from the frontend: typing is the worst
	// thing a wall asks anyone to do, so the presets are most of what the
	// control is — and they are read out by a voice that speaks one
	// language, which the household picks. Edited in the full app; the
	// kiosk only reads them (§16).
	presets := store.ViewValue(s.Store, func() []string { return s.Store.Settings.Presets() })

	writeJSON(w, http.StatusOK, map[string]any{
		"available": len(rooms) > 0,
		"rooms":     list,
		// voice false means the announcement is a chime with no words. The
		// panel says so rather than letting someone type a sentence that
		// nobody will hear.
		"voice":    announce.VoiceFromEnv() != nil,
		"max_text": announce.MaxTextLen,
		"presets":  presets,
	})
}

// announceTarget is one room an announcement can reach.
type announceTarget struct {
	ID   string
	Name string
	IP   string
}

// announceTargets is every reachable Sonos coordinator, one per group.
//
// Reachability comes from the event monitor's cache rather than a fresh
// probe: it is what the status poll already knows, and an announcement should
// not begin with a round of network discovery while someone stands at the
// wall waiting.
func (s *Server) announceTargets() []announceTarget {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap := s.sonosEvents().Snapshot(ctx)

	coordinators := make(map[string]bool, len(snap.Groups))
	byUUID := map[string]string{}
	speakers := store.ViewValue(s.Store, func() []store.SonosSpeaker {
		out := make([]store.SonosSpeaker, 0, len(s.Store.Sonos))
		for _, sp := range s.Store.Sonos {
			out = append(out, *sp)
		}
		return out
	})
	for _, sp := range speakers {
		if sp.UUID != "" {
			byUUID[sp.UUID] = sp.ID
		}
	}
	for _, g := range snap.Groups {
		if id, ok := byUUID[g.CoordinatorUUID]; ok {
			coordinators[id] = true
		}
	}

	out := make([]announceTarget, 0, len(speakers))
	for _, sp := range speakers {
		if !snap.Speakers[sp.ID].Reachable || sp.IP == "" {
			continue
		}
		// With no topology at all (a household whose groups couldn't be
		// read) every reachable speaker is treated as its own coordinator,
		// which is what a standalone speaker is anyway.
		if len(coordinators) > 0 && !coordinators[sp.ID] {
			continue
		}
		out = append(out, announceTarget{ID: sp.ID, Name: sp.Name, IP: sp.IP})
	}
	return out
}

// announceSend handles POST /api/announce with {"text": "Dinner's ready"}.
//
// The two halves are split across the response on purpose. Everything that
// can fail in a way the person at the panel can act on — nothing reachable,
// no address configured, a speaker that refused the clip — happens before the
// response, so the tap gets a real answer. The restore happens after it, in
// the background: it takes as long as the clip does, and a wall panel that
// blocks for six seconds on a tap reads as broken.
func (s *Server) announceSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
		// Rooms, by ID, to narrow the announcement to. Empty (the panel's
		// default) means every reachable room.
		Rooms []string `json:"rooms"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if len([]rune(body.Text)) > announce.MaxTextLen {
		writeError(w, http.StatusBadRequest, "that announcement is too long to read out")
		return
	}

	// One at a time, household-wide. A second announcement that starts
	// while the first is still playing would snapshot the *clip* as what
	// the room was doing — and then "restore" every room to a dead clip
	// URL at announcement volume, with the music gone for good. The busy
	// state on the panel's button covers the request, which is over in a
	// second; this covers the several seconds of audio after it.
	if !s.announceBegin() {
		writeError(w, http.StatusConflict, "the house is already being announced to — give it a moment")
		return
	}
	// Released here only on the paths that never reached a speaker; once
	// rooms are playing it belongs to the restore goroutine.
	release := s.announceEnd
	defer func() {
		if release != nil {
			release()
		}
	}()

	targets := s.announceTargets()
	if len(body.Rooms) > 0 {
		want := make(map[string]bool, len(body.Rooms))
		for _, id := range body.Rooms {
			want[id] = true
		}
		picked := make([]announceTarget, 0, len(targets))
		for _, t := range targets {
			if want[t.ID] {
				picked = append(picked, t)
			}
		}
		targets = picked
	}
	if len(targets) == 0 {
		writeError(w, http.StatusConflict, "no speaker is answering, so there is nowhere to announce")
		return
	}
	host := s.announceHost()
	if host == nil {
		writeError(w, http.StatusConflict,
			"this server has no address the speakers can reach — set HOMEHUB_STREAM_URL")
		return
	}

	// A voice failure is not an announcement failure: Build hands back a
	// usable chime either way, and the room still gets called.
	buildCtx, cancelBuild := context.WithTimeout(r.Context(), 25*time.Second)
	clip, voiceErr := announce.Build(buildCtx, announce.VoiceFromEnv(), body.Text)
	cancelBuild()
	spoken := voiceErr == nil
	if voiceErr != nil && body.Text != "" {
		log.Printf("announce: falling back to the chime: %v", voiceErr)
	}

	url, err := host.Publish(clip)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// Snapshot and start, in parallel across rooms: an announcement that
	// reaches the kitchen a second after the hallway is two announcements.
	type started struct {
		target announceTarget
		snap   *sonos.TransportSnapshot
	}
	var (
		mu      sync.Mutex
		running []started
		failed  []string
	)
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(t announceTarget) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*sonos.DefaultTimeout)
			defer cancel()
			// The snapshot comes first and its failure is fatal for this
			// room: interrupting a room we cannot put back is the one
			// thing this feature must never do.
			snap, err := sonos.SnapshotTransport(ctx, t.IP)
			if err != nil {
				mu.Lock()
				failed = append(failed, t.Name)
				mu.Unlock()
				return
			}
			if err := sonos.PlayClip(ctx, t.IP, url, announceVolume); err != nil {
				// It may have got as far as changing the volume or the
				// URI, so put it back rather than leaving it mid-way.
				_ = sonos.RestoreTransport(ctx, t.IP, snap)
				mu.Lock()
				failed = append(failed, t.Name)
				mu.Unlock()
				return
			}
			mu.Lock()
			running = append(running, started{target: t, snap: snap})
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	if len(running) == 0 {
		writeError(w, http.StatusBadGateway, "no room accepted the announcement")
		return
	}

	// Hand every room back once the clip has played. Detached from the
	// request: the caller has its answer, and this must finish even if they
	// walk away from the panel.
	wait := clip.Duration() + announceTail
	release = nil // the restore below owns it now
	go func() {
		defer s.announceEnd()
		time.Sleep(wait)
		ctx, cancel := context.WithTimeout(context.Background(), announceRestoreBudget)
		defer cancel()
		var rwg sync.WaitGroup
		for _, run := range running {
			rwg.Add(1)
			go func(run started) {
				defer rwg.Done()
				if err := sonos.RestoreTransport(ctx, run.target.IP, run.snap); err != nil {
					log.Printf("announce: putting %s back: %v", run.target.Name, err)
				}
			}(run)
		}
		rwg.Wait()
		// The rooms have moved twice without the monitor being told, so
		// nudge it: without this the panel shows the announcement's own
		// (empty) now-playing until the next poll lands.
		s.sonosEvents().Nudge()
	}()

	names := make([]string, 0, len(running))
	for _, run := range running {
		names = append(names, run.target.Name)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"rooms": names,
		// Which rooms didn't take it, so the panel can say "everywhere but
		// the bedroom" rather than implying the whole house heard it.
		"unreachable": failed,
		"spoken":      spoken,
		"duration_ms": wait.Milliseconds(),
	})
}
