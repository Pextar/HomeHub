package api

// "What am I actually listening to, and can I do better?"
//
// Two endpoints. The GET answers the question for every path audio can take
// through this house, so a user can see that the answer differs by route
// before they wonder why one room sounds different from another. The PUT
// changes the one thing that is genuinely changeable.
//
// The restraint here is the point. There is exactly one lever — how hard
// HomeHub's own decoder asks the service to compress — and it only moves the
// two routes HomeHub decodes for. Every other stage is somebody else's:
// Spotify's catalogue is lossy at source and no setting in this app changes
// that, and a speaker streaming from its own account link negotiates a bitrate
// it never tells us. Saying so plainly is the feature. A quality screen full
// of controls that quietly do nothing would be worse than no screen at all.

import (
	"net/http"

	"homehub/internal/media"
	"homehub/internal/store"
)

// qualityOption is one choice in the picker, with what it actually does.
type qualityOption struct {
	Value       media.StreamQuality `json:"value"`
	Label       string              `json:"label"`
	BitrateKbps int                 `json:"bitrate_kbps"`
	Detail      string              `json:"detail"`
}

// qualityOptions is the picker, best first. The detail lines say what each
// choice costs and gains in the terms the choice is actually made in —
// bandwidth against fidelity — rather than restating the label.
func qualityOptions() []qualityOption {
	return []qualityOption{
		{
			Value: media.QualityBest, Label: media.QualityBest.Label(),
			BitrateKbps: media.QualityBest.Bitrate(),
			Detail: "The most the service will give. About 1.4 Mbit/s per speaker " +
				"once HomeHub re-serves it, which is nothing on a home network.",
		},
		{
			Value: media.QualityBalanced, Label: media.QualityBalanced.Label(),
			BitrateKbps: media.QualityBalanced.Bitrate(),
			Detail:      "Half the source bitrate. Worth choosing if your network is congested.",
		},
		{
			Value: media.QualitySaver, Label: media.QualitySaver.Label(),
			BitrateKbps: media.QualitySaver.Bitrate(),
			Detail:      "The lowest the service offers. Audible on good speakers.",
		},
	}
}

// mediaQuality handles GET /api/media/quality.
func (s *Server) mediaQuality(w http.ResponseWriter, r *http.Request) {
	pref := s.Audio.Quality()

	// Every route, not just the one some zone happens to take. The routes
	// differ in whether this setting reaches them at all, and showing them
	// side by side is what makes that visible instead of surprising.
	type routeChain struct {
		Route media.Route `json:"route"`
		Label string      `json:"label"`
		// Decoded marks the routes where HomeHub holds the audio, which are
		// exactly the ones the setting below moves.
		Decoded bool        `json:"decoded"`
		Chain   media.Chain `json:"chain"`
	}
	type providerQuality struct {
		ID     string       `json:"id"`
		Name   string       `json:"name"`
		Routes []routeChain `json:"routes"`
	}

	provs := s.Music.Providers()
	out := struct {
		StreamQuality media.StreamQuality `json:"stream_quality"`
		Bitrate       int                 `json:"bitrate_kbps"`
		Options       []qualityOption     `json:"options"`
		Providers     []providerQuality   `json:"providers"`
	}{
		StreamQuality: pref,
		Bitrate:       pref.Bitrate(),
		Options:       qualityOptions(),
		Providers:     make([]providerQuality, 0, len(provs)),
	}

	for _, p := range provs {
		pq := providerQuality{ID: p.ID(), Name: p.Name()}
		for _, route := range p.Routes() {
			decoded := route == media.RouteStream || route == media.RouteAirPlay
			pq.Routes = append(pq.Routes, routeChain{
				Route:   route,
				Label:   routeDisplayName(route),
				Decoded: decoded,
				Chain:   media.DescribeQuality(p, route, pref),
			})
		}
		out.Providers = append(out.Providers, pq)
	}
	writeJSON(w, http.StatusOK, out)
}

// mediaSetQuality handles PUT /api/media/quality with {"stream_quality":"best"}.
//
// The change lands on the next thing started, not on what is playing. The
// bitrate is baked into the decoder's command line, so applying it now would
// mean killing a running decode — cutting off the music to improve it, which
// nobody asked for. The decoder is rebuilt at the next play; see decoder().
func (s *Server) mediaSetQuality(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StreamQuality string `json:"stream_quality"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var applied media.StreamQuality
	if !s.update(w, func() error {
		if s.Store.Settings == nil {
			s.Store.Settings = &store.Settings{}
		}
		merged := *s.Store.Settings
		merged.StreamQuality = body.StreamQuality
		if err := s.Store.ValidateSettings(&merged); err != nil {
			return errInvalid(err)
		}
		*s.Store.Settings = merged
		applied = media.StreamQuality(merged.StreamQuality).Normalize()
		return nil
	}) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stream_quality": applied,
		"bitrate_kbps":   applied.Bitrate(),
		// Said in the response rather than left for the user to notice: a
		// setting that appears to take effect and doesn't is worse than one
		// that says when it will.
		"applies": "from the next thing you play",
	})
}

// routeDisplayName is a route in the words the Music view uses for it.
func routeDisplayName(r media.Route) string {
	switch r {
	case media.RouteNative:
		return "Speaker's own stream"
	case media.RouteGroup:
		return "Grouped speakers"
	case media.RouteConnect:
		return "Spotify Connect"
	case media.RouteAirPlay:
		return "AirPlay"
	case media.RouteStream:
		return "HomeHub stream"
	}
	return string(r)
}
