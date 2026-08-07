package sonos

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Interrupting a room to say something, and putting it back the way it was.
//
// Sonos has no "play this clip" action: a speaker plays one transport URI at
// a time, so an announcement means taking the room over and handing it back.
// That makes the snapshot the whole feature — anything it fails to capture is
// something the household loses when someone calls the kids for dinner, which
// is a far worse outcome than the announcement not playing at all.
//
// Four things are captured, because four things are lost otherwise: the
// transport URI (what the room was on), its metadata (without which a
// restored radio stream comes back nameless), the queue position and the
// elapsed time (a record resumes where it was, not at the top), and the
// volume (an announcement is louder on purpose). What is deliberately *not*
// captured is per-member volume: the group volume call preserves the relative
// balance between speakers, so restoring the group's level restores the mix
// with it, and reading five speakers to write five back would triple the
// calls in the window where the room is silent.

// TransportSnapshot is a room mid-thought: everything needed to put it back.
type TransportSnapshot struct {
	// URI and Metadata are the transport itself. An empty URI means the
	// room was on nothing, which restores as "stay stopped".
	URI      string
	Metadata string
	// Track is the 1-based queue position, 0 when the source isn't a queue.
	Track int
	// Position is the elapsed time, empty for a live stream (which has no
	// position to return to — it will come back at "now", correctly).
	Position string
	// State is what the transport was doing: PLAYING, PAUSED_PLAYBACK,
	// STOPPED. A room that was paused must not come back playing.
	State string
	// GroupVolume is the group's level as GroupRenderingControl reports it,
	// which is the one number that restores the whole group's balance.
	GroupVolume int
}

// Playing reports whether the room was making noise when it was interrupted.
func (t *TransportSnapshot) Playing() bool {
	return t != nil && (t.State == "PLAYING" || t.State == "TRANSITIONING")
}

// SnapshotTransport reads everything RestoreTransport needs. Send to the
// group coordinator.
//
// Sub-reads degrade rather than fail: a room that cannot say its position
// restores at the top of the track, which is worse than exact and much
// better than not restoring at all.
func SnapshotTransport(ctx context.Context, ip string) (*TransportSnapshot, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetMediaInfo", []arg{{"InstanceID", instance0}})
	if err != nil {
		return nil, err
	}
	snap := &TransportSnapshot{
		URI:      extractTag(body, "CurrentURI"),
		Metadata: extractTag(body, "CurrentURIMetaData"),
	}
	if body, err := soapCall(ctx, ip, avTransport, "GetTransportInfo",
		[]arg{{"InstanceID", instance0}}); err == nil {
		snap.State = extractTag(body, "CurrentTransportState")
	}
	if body, err := soapCall(ctx, ip, avTransport, "GetPositionInfo",
		[]arg{{"InstanceID", instance0}}); err == nil {
		snap.Track, _ = strconv.Atoi(extractTag(body, "Track"))
		snap.Position = normalizeClock(extractTag(body, "RelTime"))
	}
	if v, err := GetGroupVolume(ctx, ip); err == nil {
		snap.GroupVolume = v
	}
	return snap, nil
}

// GetGroupVolume reads the group's volume. Must be sent to the coordinator.
func GetGroupVolume(ctx context.Context, ip string) (int, error) {
	body, err := soapCall(ctx, ip, groupRendering, "GetGroupVolume",
		[]arg{{"InstanceID", instance0}})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(extractTag(body, "CurrentVolume"))
}

// PlayClip takes the room over with one audio URL, at the given group
// volume. The URL must be something the speaker can fetch over plain HTTP on
// the LAN — a Sonos will not follow a redirect to HTTPS or accept a
// self-signed certificate.
//
// A clip is set as the transport URI rather than enqueued: it is a single
// short file with nothing after it, and putting it in the queue would both
// disturb the queue and leave the room trying to advance past it.
func PlayClip(ctx context.Context, ip, url string, volume int) error {
	if strings.TrimSpace(url) == "" {
		return errNoClipURL
	}
	// Volume first: setting it after the URI means the first word of the
	// announcement plays at whatever the room was on, which on a room that
	// was silent at 4% is the entire point missed.
	if volume > 0 {
		if err := SetGroupVolume(ctx, ip, volume); err != nil {
			return err
		}
	}
	if err := SetAVTransportURI(ctx, ip, url, ""); err != nil {
		return err
	}
	return Play(ctx, ip)
}

// RestoreTransport puts a room back the way SnapshotTransport found it.
//
// Errors are collected rather than returned early: every step that can still
// run should, because a room left on a finished announcement URI is the one
// outcome nobody can undo from the panel. The first failure is reported once
// everything else has been attempted.
func RestoreTransport(ctx context.Context, ip string, snap *TransportSnapshot) error {
	if snap == nil {
		return nil
	}
	var firstErr error
	fail := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if snap.GroupVolume > 0 {
		fail(SetGroupVolume(ctx, ip, snap.GroupVolume))
	}
	if strings.TrimSpace(snap.URI) == "" {
		// Nothing was on. Stopping is what puts the room back to silence —
		// without it the room sits on a played-out clip URI.
		_, err := soapCall(ctx, ip, avTransport, "Stop", []arg{{"InstanceID", instance0}})
		fail(err)
		return firstErr
	}

	fail(SetAVTransportURI(ctx, ip, snap.URI, snap.Metadata))
	// A queue URI comes back at track one; the queue position is a separate
	// fact and has to be re-stated.
	if snap.Track > 1 && strings.HasPrefix(snap.URI, "x-rincon-queue:") {
		fail(SeekTrack(ctx, ip, snap.Track))
	}
	if snap.Position != "" {
		// Best-effort: a stream refuses this, and a refused seek must not
		// stop the room from resuming.
		_ = Seek(ctx, ip, snap.Position)
	}
	if snap.Playing() {
		fail(Play(ctx, ip))
	}
	return firstErr
}

// errNoClipURL is a package-level error so a caller can tell "nothing to
// play" from a speaker that refused.
var errNoClipURL = errors.New("sonos: announcement has no URL to play")
