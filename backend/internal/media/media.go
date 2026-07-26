// Package media is the vendor-neutral layer between music services and
// speakers. It exists so that "play this on those speakers" is one code path
// rather than one per bridge, and so the cross-vendor case — a KEF and a Sonos
// playing the same thing at once — has somewhere to live at all.
//
// The model is four nouns, documented in full in docs/MEDIA-PROTOCOL.md:
//
//	Provider  a music service (Spotify), which can search and can serve
//	          content over one or more routes
//	Endpoint  one speaker, addressed uniformly; adapters wrap the existing
//	          internal/sonos and internal/kef bridges
//	Zone      a named set of endpoints that play together, of any mix of
//	          vendors; a zone of one is the ordinary single-speaker case
//	Route     how content gets from a provider onto a zone
//
// Nothing here talks to hardware. The adapters do that, and they translate
// only — an adapter never emulates a capability the speaker lacks, because a
// capability that lies is worse than one that is absent: the route engine
// picks paths based on these declarations, and a wrong pick starts music in
// the wrong room.
package media

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Vendor identifies which bridge is behind an endpoint. It is deliberately
// coarse: the route engine uses it only to decide whether two endpoints can
// group natively with each other, never to special-case behaviour. Anything
// finer belongs in Capability.
type Vendor string

const (
	VendorSonos Vendor = "sonos"
	VendorKEF   Vendor = "kef"
)

// Capability is one thing an endpoint can do. Endpoints declare a set of
// these, and the route engine reasons entirely in terms of them — that is what
// lets a new bridge join without the engine learning about it.
//
// A capability describes the *hardware and its bridge*, not the current
// moment: a KEF in standby still has CapConnect, it just needs waking first.
// Conditions that change minute to minute are reported by Availability, not
// by dropping a capability.
type Capability uint32

const (
	// CapTransport is play/pause/next/previous. Every endpoint has it.
	CapTransport Capability = 1 << iota
	// CapVolume is 0-100 volume and mute.
	CapVolume
	// CapSeek is seeking within the current track.
	CapSeek
	// CapQueue is inspecting and mutating a queue of upcoming tracks.
	CapQueue
	// CapGroup is native multi-speaker grouping with same-vendor endpoints,
	// where the vendor's own clock keeps them in sync.
	CapGroup
	// CapPlayURI is being handed an arbitrary stream URL. This is the one
	// that makes the stream route possible, and it is the only capability
	// both vendors share for *starting* content.
	CapPlayURI
	// CapNativeService is streaming a music service directly, from an
	// account link held by the speaker itself. Sonos has this; KEF does not,
	// which is the asymmetry the whole route engine exists to paper over.
	CapNativeService
	// CapConnect is being targeted by a service's own cloud (Spotify
	// Connect). The inverse asymmetry: KEF has it, Sonos does not.
	CapConnect
	// CapWake is being woken from standby onto the network. Endpoints
	// without it are always reachable; endpoints with it must be woken
	// before any route that needs them to exist on the network.
	CapWake
)

// capNames drives Has, String and the JSON encoding. Kept in declaration
// order so a printed set reads consistently.
var capNames = []struct {
	cap  Capability
	name string
}{
	{CapTransport, "transport"},
	{CapVolume, "volume"},
	{CapSeek, "seek"},
	{CapQueue, "queue"},
	{CapGroup, "group"},
	{CapPlayURI, "play_uri"},
	{CapNativeService, "native_service"},
	{CapConnect, "connect"},
	{CapWake, "wake"},
}

// Has reports whether every capability in want is present. Passing a
// multi-bit want asks for all of them, which is what route predicates need.
func (c Capability) Has(want Capability) bool { return c&want == want }

// Names returns the set as stable strings, for the API and for error
// messages that have to tell a user what a speaker can actually do.
func (c Capability) Names() []string {
	var out []string
	for _, n := range capNames {
		if c&n.cap != 0 {
			out = append(out, n.name)
		}
	}
	return out
}

func (c Capability) String() string {
	names := c.Names()
	if len(names) == 0 {
		return "none"
	}
	return fmt.Sprint(names)
}

// MarshalJSON encodes a capability set as an array of names rather than an
// integer, so the frontend can check for a capability without a shared table
// of bit values that would drift.
func (c Capability) MarshalJSON() ([]byte, error) {
	names := c.Names()
	if names == nil {
		names = []string{}
	}
	// Hand-rolled: the names are a closed set of safe identifiers, so this
	// avoids pulling encoding/json into a hot path for no benefit.
	buf := make([]byte, 0, 16*len(names)+2)
	buf = append(buf, '[')
	for i, n := range names {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, n...)
		buf = append(buf, '"')
	}
	return append(buf, ']'), nil
}

// Descriptor is an endpoint's identity and what it can do — everything the
// route engine and the UI need without touching the device.
type Descriptor struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Room   string     `json:"room,omitempty"`
	Vendor Vendor     `json:"vendor"`
	Model  string     `json:"model,omitempty"`
	Caps   Capability `json:"capabilities"`
	// GroupKey is the set within which CapGroup endpoints can natively
	// group. Same vendor is necessary but not sufficient — two Sonos
	// households on one LAN can't group with each other — so the adapter
	// decides what identifies a group domain rather than the engine
	// assuming vendor equality is enough.
	GroupKey string `json:"-"`
}

// PlayState is the normalised transport state. The bridges disagree on
// vocabulary (Sonos says PLAYING/PAUSED_PLAYBACK/STOPPED/TRANSITIONING, KEF
// says playing/paused/stopped) and both are preserved on their own endpoints;
// this is the common denominator the zone layer reports.
type PlayState string

const (
	StatePlaying       PlayState = "playing"
	StatePaused        PlayState = "paused"
	StateStopped       PlayState = "stopped"
	StateTransitioning PlayState = "transitioning"
)

// Track is the currently playing item, as much of it as the endpoint knows.
type Track struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	// ArtURI may be absolute or relative to the endpoint. Sonos serves
	// artwork off the speaker itself (/getaa?...), so a relative value has
	// to be proxied by the caller — the same rule the sonos bridge already
	// documents, carried through rather than resolved here, because this
	// package has no HTTP client and should not grow one.
	ArtURI string `json:"art_uri,omitempty"`
}

// NowPlaying is one endpoint's live state, normalised.
type NowPlaying struct {
	State  PlayState `json:"state"`
	Volume int       `json:"volume"` // 0-100
	Muted  bool      `json:"muted"`
	Track  *Track    `json:"track,omitempty"`
	// Position and Duration are zero when the endpoint doesn't report them;
	// Duration is also zero for live streams, where there is nothing to
	// seek. Durations rather than the bridges' strings/millis so callers
	// stop reimplementing the parse.
	Position time.Duration `json:"-"`
	Duration time.Duration `json:"-"`
	// PositionMS/DurationMS are the wire form. Milliseconds because that is
	// what both the KEF bridge and the frontend already use.
	PositionMS int64 `json:"position_ms,omitempty"`
	DurationMS int64 `json:"duration_ms,omitempty"`
	// At is when this reading was taken, so a client extrapolating position
	// knows how stale its starting point is.
	At time.Time `json:"at"`
}

// SyncWire fills the millisecond fields from the Duration ones. Adapters set
// the typed fields and call this, so the two representations cannot drift.
func (n *NowPlaying) SyncWire() {
	n.PositionMS = n.Position.Milliseconds()
	n.DurationMS = n.Duration.Milliseconds()
}

// Playing reports whether audio is actually coming out, which is not the same
// as "not stopped" — transitioning is neither.
func (n *NowPlaying) Playing() bool { return n != nil && n.State == StatePlaying }

// Endpoint is one speaker. Implementations wrap a bridge and translate; see
// the package doc on why they must not emulate.
//
// Every method takes a context and performs device I/O. None of them may be
// called while store.Mu is held — that is the rule from CLAUDE.md, and the
// zone layer's staged execution is what enforces it.
type Endpoint interface {
	Descriptor() Descriptor
	State(ctx context.Context) (*NowPlaying, error)

	Play(ctx context.Context) error
	Pause(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	SetVolume(ctx context.Context, level int) error
	SetMute(ctx context.Context, muted bool) error
}

// The optional interfaces below correspond to capabilities. A caller that
// holds a capability may type-assert to the matching interface and expect it
// to succeed; adapters must keep the two in step. Declaring CapSeek without
// implementing Seeker is a bug in the adapter, and assertCaps catches it in
// tests.

// Seeker is CapSeek.
type Seeker interface {
	Seek(ctx context.Context, pos time.Duration) error
}

// QueueItem is one entry in an endpoint's queue.
type QueueItem struct {
	Position int    `json:"position"` // 1-based
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	ArtURI   string `json:"art_uri,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// Queuer is CapQueue.
type Queuer interface {
	Queue(ctx context.Context) ([]QueueItem, error)
	ClearQueue(ctx context.Context) error
}

// Grouper is CapGroup. Join and Leave address the vendor's native grouping;
// the route engine only ever calls them between endpoints sharing a GroupKey.
type Grouper interface {
	Join(ctx context.Context, coordinator Endpoint) error
	Leave(ctx context.Context) error
	// Coordinator reports the endpoint currently leading this one's group,
	// or "" when it stands alone. Used to avoid re-grouping speakers that
	// are already arranged correctly, which would interrupt playback.
	Coordinator(ctx context.Context) (string, error)
}

// Metadata is what a URI player should display while playing a stream. All
// fields are advisory: an endpoint shows what it can.
type Metadata struct {
	Title  string
	Artist string
	Album  string
	ArtURI string
	// Live marks a stream with no end, which changes how Sonos presents it
	// (no scrubber, no track duration) and stops it trying to advance.
	Live bool
}

// URIPlayer is CapPlayURI — the capability the stream route is built on.
type URIPlayer interface {
	PlayURI(ctx context.Context, uri string, meta Metadata) error
}

// NativeServicePlayer is CapNativeService: hand the speaker a service URI and
// let it stream from its own account link. The account token comes from the
// provider, which is why the argument is opaque here.
type NativeServicePlayer interface {
	// PlayNative starts service content. uri and metadata are whatever the
	// provider's NativeItem produced for this vendor — the media layer
	// carries them without interpreting them.
	PlayNative(ctx context.Context, uri, metadata string) error
	// ServiceAccount fetches the speaker's link for a named service, or an
	// error when the household has no such link.
	ServiceAccount(ctx context.Context, service string) (Account, error)
}

// Account is a speaker's link to a music service, opaque to this package and
// meaningful only to the provider that issued the lookup.
type Account struct {
	// SID and Serial are the Sonos shape (service id + account serial).
	// Kept concrete rather than as `any` because there is exactly one
	// vendor with native service links, and a fake generic type here would
	// be pretending to an extensibility that isn't real yet.
	SID    int
	Serial string
	Type   int
}

// ConnectTarget is CapConnect: this endpoint can be pointed at by a service's
// cloud. DeviceHint is what the endpoint believes it is called there, which
// the provider matches against its device list.
type ConnectTarget interface {
	// ConnectHint returns a pinned device id (empty when none) and the
	// names to match on, most specific first.
	ConnectHint() (deviceID string, names []string)
}

// Waker is CapWake.
type Waker interface {
	// Wake brings the endpoint out of standby and onto its network input.
	// Must be idempotent: waking an awake endpoint is a no-op, and routes
	// call it unconditionally rather than reading state first.
	Wake(ctx context.Context) error
}

// Errors the layer defines. Callers map these to status codes; the API
// package turns the "user can fix this" ones into 409s.
var (
	// ErrNoRoute means no route can serve this zone with this provider.
	// Always wrapped with a per-endpoint explanation — see RouteError.
	ErrNoRoute = errors.New("media: no route can play this to these speakers")
	// ErrUnknownEndpoint / ErrUnknownProvider are lookup failures.
	ErrUnknownEndpoint = errors.New("media: unknown endpoint")
	ErrUnknownProvider = errors.New("media: unknown provider")
	// ErrEmptyZone is a zone with no members, which is valid to store
	// (a user emptying a zone in the UI) but not to play to.
	ErrEmptyZone = errors.New("media: zone has no speakers")
)
