package api

// The three paths devices reach rather than people.
//
// All of them sit outside /api, and none of them can carry a session cookie:
// the clients are speakers, which have no credentials and no way to acquire
// any. Each is guarded instead by something unguessable that stops mattering
// shortly after it is minted — see the handler for what, and why that is the
// best available answer.
//
// They are exported because the composition root hands them to the subsystems
// that build URLs from them: the audio engine, the announcer, and the Sonos
// event monitor all need to tell a speaker where to come back to.
const (
	// StreamPath serves the decoded audio a zone's speakers pull from.
	// Guarded by a per-playback stream id.
	StreamPath = "/stream"

	// AnnouncePath serves the clip a speaker fetches when the house is
	// being called. Guarded by a per-clip id that expires in minutes.
	AnnouncePath = "/announce"

	// SonosEventPath receives GENA change notifications. Guarded by a
	// per-speaker token, the subscription id we issued, and a check that
	// the request came from that speaker's own address.
	SonosEventPath = "/sonos/event"
)
