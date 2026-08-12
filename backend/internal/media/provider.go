package media

import (
	"context"
	"io"
)

// ItemKind is what a search result is. The route engine cares because not
// every route can carry every kind: Sonos takes containers through the queue,
// Spotify Connect takes a context URI, and the stream route only ever plays
// whatever the decoder was handed.
type ItemKind string

const (
	KindTrack    ItemKind = "track"
	KindAlbum    ItemKind = "album"
	KindPlaylist ItemKind = "playlist"
	KindArtist   ItemKind = "artist"
	KindStation  ItemKind = "station"
)

// Item is one piece of content, uniform across services. URI is the
// provider's own canonical identifier (spotify:track:…) and is opaque to
// everything except the provider that issued it.
type Item struct {
	Provider string   `json:"provider"`
	Kind     ItemKind `json:"kind"`
	URI      string   `json:"uri"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"` // artist line, owner, …
	ArtURI   string   `json:"art_uri,omitempty"`
}

// Results is a search response, grouped by kind so a UI can present sections
// without re-bucketing. Empty slices rather than nil so the JSON is stable.
type Results struct {
	Tracks    []Item `json:"tracks"`
	Albums    []Item `json:"albums"`
	Playlists []Item `json:"playlists"`
	Artists   []Item `json:"artists"`
}

// Availability is whether a provider can be used right now, and if not, what
// the user should do about it. Reason is shown verbatim, so it is written as
// a sentence aimed at a person rather than a log line.
type Availability struct {
	OK bool `json:"ok"`
	// Configured distinguishes "you never set this up" from "your session
	// expired", which need different prompts.
	Configured bool   `json:"configured"`
	Reason     string `json:"reason,omitempty"`
}

// Provider is a music service. Search and browse are uniform; how content
// actually reaches a speaker is not, so a provider declares which routes it
// can serve and implements the matching optional interface below.
type Provider interface {
	ID() string   // stable slug, "spotify"
	Name() string // display name, "Spotify"
	Available() Availability
	Search(ctx context.Context, query string, limit int) (*Results, error)
	// Browse is the no-typing entry point: the user's own playlists,
	// favorites, whatever the service offers as a starting point.
	Browse(ctx context.Context, limit int) ([]Item, error)
	// Routes is the set this provider can serve, which bounds route
	// selection from the service side just as capabilities bound it from
	// the speaker side.
	Routes() RouteSet
}

// NativeProvider serves RouteNative: the speaker streams the content itself
// using its own account link.
type NativeProvider interface {
	// NativeItem maps a provider URI to the vendor-specific URI + metadata
	// a speaker needs, for the account the speaker holds. The strings are
	// passed through NativeServicePlayer.PlayNative untouched.
	NativeItem(vendor Vendor, item Item, acct Account) (uri, metadata string, err error)
	// ServiceName is what this provider is called in the vendor's service
	// list, which is not always its display name.
	ServiceName(vendor Vendor) string
}

// ConnectDevice is one device the provider's cloud can target.
type ConnectDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	// Active is whether the service is currently playing to it, and
	// Restricted whether the service refuses third-party control — a
	// restricted device matches by name but would silently ignore the
	// command, so it is rejected with a reason instead.
	Active     bool `json:"active"`
	Restricted bool `json:"restricted"`
}

// ConnectProvider serves RouteConnect: the service's own cloud is asked to
// play on a device that is the speaker.
type ConnectProvider interface {
	ConnectDevices(ctx context.Context) ([]ConnectDevice, error)
	PlayOn(ctx context.Context, deviceID string, item Item) error
}

// Stream is decoded audio plus the metadata to display alongside it.
type Stream struct {
	// Body is the encoded audio, read until the session ends. Closing it
	// releases the decoder.
	Body io.ReadCloser
	// ContentType is what to advertise to endpoints ("audio/flac",
	// "audio/mpeg"). Endpoints differ in what they accept, so the transport
	// picks per listener and this is the default.
	//
	// It describes what a *listener* will be served, which is not always
	// the bytes in Body: the WAV transport prepends a header per listener
	// rather than expecting one in the stream. PCM says what Body itself
	// is.
	ContentType string
	// PCM, when set, says Body is raw uncompressed samples in this format —
	// no container, no header, nothing to parse. The AirPlay route needs
	// that: it packs samples into RTP packets and cannot skip past a header
	// it did not expect, and a route that guessed would put a container's
	// first bytes through a speaker as noise.
	PCM *PCMFormat
	// Meta is what the endpoints should display. It is a snapshot at open
	// time; live updates come over the transport's metadata channel.
	Meta Metadata
}

// PCMFormat describes raw samples.
type PCMFormat struct {
	SampleRate int
	BitDepth   int
	Channels   int
	// LittleEndian is how the samples are ordered in Body. Worth stating
	// rather than assuming: decoders write host order, and the wire formats
	// that carry PCM are big-endian.
	LittleEndian bool
}

// CDQuality is 44.1 kHz 16-bit stereo little-endian — what every decoder in
// this codebase produces and what AirPlay 1 carries.
var CDQuality = PCMFormat{SampleRate: 44100, BitDepth: 16, Channels: 2, LittleEndian: true}

// Matches reports whether f is the same format as want.
func (f *PCMFormat) Matches(want PCMFormat) bool {
	return f != nil && *f == want
}

// StreamProvider serves RouteStream: HomeHub decodes the content once and
// re-serves it, which is the only way to get one service onto speakers of
// different vendors simultaneously. See docs/MEDIA-PROTOCOL.md on why nothing
// cheaper works.
type StreamProvider interface {
	// OpenStream begins decoding and returns the audio. The provider owns
	// whatever process does the decoding; the caller owns closing the
	// stream.
	OpenStream(ctx context.Context, item Item) (*Stream, error)
	// StreamAvailable reports whether the decoder is actually usable —
	// librespot present, account Premium — separately from Available(),
	// because a provider can be perfectly good for search and unable to
	// stream.
	StreamAvailable() Availability
}
