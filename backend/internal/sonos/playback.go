package sonos

// Transport capabilities beyond play/pause/skip: seeking, play modes
// (shuffle × repeat), crossfade, and the group queue.
//
// All of these address the *group*, so every call here must be sent to the
// coordinator — a member speaker will either refuse or silently affect only
// itself. Callers resolve the coordinator before reaching this file.

import (
	"context"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ── Seeking ──────────────────────────────────────────────────────────────

// clockRe matches the H:MM:SS / HH:MM:SS form UPnP uses for REL_TIME.
var clockRe = regexp.MustCompile(`^\d{1,2}:[0-5]\d:[0-5]\d$`)

// Seek jumps to an absolute position within the current track. position is
// "H:MM:SS". Sources without a duration (radio, line-in) reject this — the
// speaker's UPnP error is surfaced as-is.
func Seek(ctx context.Context, ip, position string) error {
	if !clockRe.MatchString(position) {
		return fmt.Errorf("sonos: %q is not a H:MM:SS position", position)
	}
	_, err := soapCall(ctx, ip, avTransport, "Seek",
		[]arg{{"InstanceID", instance0}, {"Unit", "REL_TIME"}, {"Target", position}})
	return err
}

// SeekTrack jumps to a 1-based track number in the group queue.
func SeekTrack(ctx context.Context, ip string, track int) error {
	if track < 1 {
		return fmt.Errorf("sonos: track number must be 1 or greater, got %d", track)
	}
	_, err := soapCall(ctx, ip, avTransport, "Seek",
		[]arg{{"InstanceID", instance0}, {"Unit", "TRACK_NR"}, {"Target", strconv.Itoa(track)}})
	return err
}

// ── Play modes ───────────────────────────────────────────────────────────

// Repeat modes, as exposed to the API. Sonos encodes shuffle and repeat in
// one composite string; we split them into the two axes the UI actually
// presents as separate controls.
const (
	RepeatOff = "off"
	RepeatAll = "all"
	RepeatOne = "one"
)

// PlayMode is the shuffle/repeat pair for one group.
type PlayMode struct {
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"` // off | all | one
}

// playModes maps every Sonos play-mode string onto its axes. SHUFFLE means
// "shuffle + repeat all" — a long-standing quirk of the protocol, not a typo.
var playModes = map[string]PlayMode{
	"NORMAL":             {false, RepeatOff},
	"REPEAT_ALL":         {false, RepeatAll},
	"REPEAT_ONE":         {false, RepeatOne},
	"SHUFFLE_NOREPEAT":   {true, RepeatOff},
	"SHUFFLE":            {true, RepeatAll},
	"SHUFFLE_REPEAT_ONE": {true, RepeatOne},
}

// ParsePlayMode maps a Sonos play-mode string onto its axes. Unknown values
// — newer firmware could add one — degrade to the neutral mode rather than
// erroring, since this is read on every status poll.
func ParsePlayMode(s string) PlayMode {
	if m, ok := playModes[strings.ToUpper(strings.TrimSpace(s))]; ok {
		return m
	}
	return PlayMode{Repeat: RepeatOff}
}

// String renders the pair back into the Sonos play-mode string.
func (m PlayMode) String() string {
	for name, mode := range playModes {
		if mode == m {
			return name
		}
	}
	return "NORMAL"
}

// Valid reports whether Repeat holds one of the three accepted values.
func (m PlayMode) Valid() bool {
	return m.Repeat == RepeatOff || m.Repeat == RepeatAll || m.Repeat == RepeatOne
}

// GetPlayMode reads the group's current shuffle/repeat state.
func GetPlayMode(ctx context.Context, ip string) (PlayMode, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetTransportSettings", []arg{{"InstanceID", instance0}})
	if err != nil {
		return PlayMode{Repeat: RepeatOff}, err
	}
	return ParsePlayMode(extractTag(body, "PlayMode")), nil
}

// SetPlayMode sets the group's shuffle/repeat state.
func SetPlayMode(ctx context.Context, ip string, m PlayMode) error {
	if !m.Valid() {
		return fmt.Errorf("sonos: %q is not a repeat mode", m.Repeat)
	}
	_, err := soapCall(ctx, ip, avTransport, "SetPlayMode",
		[]arg{{"InstanceID", instance0}, {"NewPlayMode", m.String()}})
	return err
}

// GetCrossfade reads whether tracks fade into each other.
func GetCrossfade(ctx context.Context, ip string) (bool, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetCrossfadeMode", []arg{{"InstanceID", instance0}})
	if err != nil {
		return false, err
	}
	return extractTag(body, "CrossfadeMode") == "1", nil
}

// SetCrossfade turns crossfading on or off.
func SetCrossfade(ctx context.Context, ip string, on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	_, err := soapCall(ctx, ip, avTransport, "SetCrossfadeMode",
		[]arg{{"InstanceID", instance0}, {"CrossfadeMode", v}})
	return err
}

// GetGroupState gathers the coordinator-level settings in one go. Partial
// failures degrade gracefully, matching GetState: a sub-request that doesn't
// answer leaves its fields zeroed rather than failing the whole read.
func GetGroupState(ctx context.Context, ip string) (*GroupState, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetTransportSettings", []arg{{"InstanceID", instance0}})
	if err != nil {
		return nil, err
	}
	m := ParsePlayMode(extractTag(body, "PlayMode"))
	gs := &GroupState{Shuffle: m.Shuffle, Repeat: m.Repeat}

	if on, err := GetCrossfade(ctx, ip); err == nil {
		gs.Crossfade = on
	}
	if body, err := soapCall(ctx, ip, avTransport, "GetMediaInfo", []arg{{"InstanceID", instance0}}); err == nil {
		gs.QueueLength, _ = strconv.Atoi(extractTag(body, "NrTracks"))
		gs.FromQueue = strings.HasPrefix(extractTag(body, "CurrentURI"), "x-rincon-queue:")
	}
	return gs, nil
}

// ── Queue ────────────────────────────────────────────────────────────────

// QueueItem is one track in the group queue.
type QueueItem struct {
	Track    int    `json:"track"` // 1-based position in the queue
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	ArtURI   string `json:"art_uri,omitempty"`
	Duration string `json:"duration,omitempty"` // H:MM:SS
}

// MaxQueueFetch caps how much of the queue we pull in one browse. Sonos
// queues can run to thousands of tracks; the UI only ever shows a window.
const MaxQueueFetch = 200

// ListQueue browses the group queue of the coordinator at ip.
func ListQueue(ctx context.Context, ip string) ([]QueueItem, error) {
	body, err := soapCall(ctx, ip, contentDirectory, "Browse", []arg{
		{"ObjectID", "Q:0"},
		{"BrowseFlag", "BrowseDirectChildren"},
		{"Filter", "*"},
		{"StartingIndex", "0"},
		{"RequestedCount", strconv.Itoa(MaxQueueFetch)},
		{"SortCriteria", ""},
	})
	if err != nil {
		return nil, err
	}
	result := extractTag(body, "Result")
	if result == "" {
		return []QueueItem{}, nil
	}
	return ParseQueue(result)
}

// queueDidl mirrors the DIDL-Lite queue listing. It duplicates a little of
// didlLite because queue rows carry a duration on the <res> element, which
// needs the attribute-bearing struct form.
type queueDidl struct {
	Items []struct {
		ID          string `xml:"id,attr"`
		Title       string `xml:"title"`
		Creator     string `xml:"creator"`
		Album       string `xml:"album"`
		AlbumArtURI string `xml:"albumArtURI"`
		Res         struct {
			Duration string `xml:"duration,attr"`
		} `xml:"res"`
	} `xml:"item"`
}

// ParseQueue parses the (unescaped) DIDL-Lite queue listing. Track numbers
// come from the item ids ("Q:0/7" → 7), which is what Seek TRACK_NR wants;
// items without a parseable id fall back to their listing order.
func ParseQueue(result string) ([]QueueItem, error) {
	var d queueDidl
	if err := xml.Unmarshal([]byte(result), &d); err != nil {
		return nil, fmt.Errorf("sonos: parse queue: %w", err)
	}
	items := make([]QueueItem, 0, len(d.Items))
	for i, it := range d.Items {
		track := i + 1
		if n := trackFromQueueID(it.ID); n > 0 {
			track = n
		}
		items = append(items, QueueItem{
			Track:    track,
			Title:    it.Title,
			Artist:   it.Creator,
			Album:    it.Album,
			ArtURI:   it.AlbumArtURI,
			Duration: normalizeClock(it.Res.Duration),
		})
	}
	return items, nil
}

// trackFromQueueID pulls the 1-based track number out of a "Q:0/7" item id.
// Returns 0 when the id doesn't have that shape.
func trackFromQueueID(id string) int {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil {
		return 0
	}
	return n
}

// QueueAdd is the outcome of enqueuing one item.
type QueueAdd struct {
	Track  int `json:"track"`  // where it landed, 1-based
	Length int `json:"length"` // queue length afterwards
}

// AddToQueue appends an item to the group queue, or drops it directly after
// the current track when next is true. Unlike PlayServiceItem this never
// clears the queue or touches the transport: what is playing keeps playing.
func AddToQueue(ctx context.Context, ip, uri, metadata string, next bool) (*QueueAdd, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("sonos: item has no URI")
	}
	// "0" means append. For "play next" we have to name the slot, which
	// takes reading where playback currently sits.
	desired := "0"
	asNext := "0"
	if next {
		asNext = "1"
		if body, err := soapCall(ctx, ip, avTransport, "GetPositionInfo",
			[]arg{{"InstanceID", instance0}}); err == nil {
			if cur, cerr := strconv.Atoi(extractTag(body, "Track")); cerr == nil && cur > 0 {
				desired = strconv.Itoa(cur + 1)
			}
		}
	}
	body, err := soapCall(ctx, ip, avTransport, "AddURIToQueue", []arg{
		{"InstanceID", instance0},
		{"EnqueuedURI", uri},
		{"EnqueuedURIMetaData", metadata},
		{"DesiredFirstTrackNumberEnqueued", desired},
		{"EnqueueAsNext", asNext},
	})
	if err != nil {
		return nil, err
	}
	add := &QueueAdd{}
	add.Track, _ = strconv.Atoi(extractTag(body, "FirstTrackNumberEnqueued"))
	add.Length, _ = strconv.Atoi(extractTag(body, "NewQueueLength"))
	return add, nil
}

// RemoveFromQueue drops one 1-based track from the group queue.
func RemoveFromQueue(ctx context.Context, ip string, track int) error {
	if track < 1 {
		return fmt.Errorf("sonos: track number must be 1 or greater, got %d", track)
	}
	_, err := soapCall(ctx, ip, avTransport, "RemoveTrackFromQueue", []arg{
		{"InstanceID", instance0},
		{"ObjectID", "Q:0/" + strconv.Itoa(track)},
		{"UpdateID", "0"},
	})
	return err
}

// ClearQueue empties the group queue. Playback stops with it.
func ClearQueue(ctx context.Context, ip string) error {
	_, err := soapCall(ctx, ip, avTransport, "RemoveAllTracksFromQueue",
		[]arg{{"InstanceID", instance0}})
	return err
}

// PlayFromQueue points the group at its own queue and starts it at track.
// Needed when the group is on another source (radio, line-in, a grouped
// coordinator) and the user picks something out of the queue: seeking alone
// would not switch the source back. speakerUUID is the coordinator's RINCON id.
func PlayFromQueue(ctx context.Context, ip, speakerUUID string, track int) error {
	if !strings.HasPrefix(speakerUUID, "RINCON_") {
		return fmt.Errorf("sonos: %q is not a Sonos device id", speakerUUID)
	}
	if _, err := soapCall(ctx, ip, avTransport, "SetAVTransportURI", []arg{
		{"InstanceID", instance0},
		{"CurrentURI", "x-rincon-queue:" + speakerUUID + "#0"},
		{"CurrentURIMetaData", ""},
	}); err != nil {
		return err
	}
	if err := SeekTrack(ctx, ip, track); err != nil {
		return err
	}
	return Play(ctx, ip)
}
