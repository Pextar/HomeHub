package app

// The two adapters the audio engine needs, and the one nil check that has to
// happen before it is handed a catalogue.
//
// They live here rather than in internal/audio because they are the seam: the
// engine must not know that a quality setting is a JSON field on a store, and
// the store must not know that anything decodes audio.

import (
	"homehub/internal/media"
	"homehub/internal/qobuz"
	"homehub/internal/store"
	"homehub/internal/stream"
)

// streamQuality reads the household's chosen decode quality.
//
// Read on every use rather than captured at startup: a household that changes
// it expects the next thing they play to honour the change, and the engine
// rebuilds its decoder when the answer moves.
func streamQuality(st *store.Store) media.StreamQuality {
	q := store.ViewValue(st, func() string {
		if st.Settings == nil {
			return ""
		}
		return st.Settings.StreamQuality
	})
	return media.StreamQuality(q).Normalize()
}

// qobuzCatalog adapts the optional Qobuz client to what the decoder needs.
//
// The nil check is load-bearing rather than defensive: assigning a nil
// *qobuz.Client straight into the interface would produce a non-nil interface
// holding a nil pointer, and every "is Qobuz configured" check downstream would
// pass on its way to a panic.
func qobuzCatalog(c *qobuz.Client) stream.Catalog {
	if c == nil {
		return nil
	}
	return c
}
