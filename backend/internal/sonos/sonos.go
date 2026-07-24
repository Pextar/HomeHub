// Package sonos provides a minimal client for the Sonos local UPnP API.
// Every Sonos speaker exposes SOAP control endpoints on port 1400 — no hub,
// no cloud account, no pairing. Playback, volume, grouping and favorites all
// work entirely on the LAN, which is what lets HomeHub stand in for the
// Sonos app for day-to-day control.
package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultTimeout caps how long we wait for a speaker to respond.
const DefaultTimeout = 5 * time.Second

// Port is the fixed port Sonos speakers listen on for UPnP control.
const Port = 1400

// ValidateHost checks that host is a bare hostname or IP that is safe to
// interpolate into http://<host>:1400/... . Mirrors tasmota.ValidateHost:
// it rejects values that could redirect the server-side request away from
// the intended device and IP literals that point at sensitive targets.
// Sonos speakers live on the LAN, so private ranges are intentionally allowed.
func ValidateHost(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return errors.New("speaker address is empty")
	}
	if strings.ContainsAny(h, "/?#@\\ \t\r\n:") {
		// Unlike Tasmota no port is accepted — Sonos is always :1400.
		return fmt.Errorf("invalid speaker address %q", host)
	}
	if parsed := net.ParseIP(h); parsed != nil {
		if parsed.IsLoopback() || parsed.IsLinkLocalUnicast() ||
			parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast() {
			return fmt.Errorf("speaker address %q is not an allowed address", host)
		}
		return nil
	}
	for _, c := range h {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '.'
		if !ok {
			return fmt.Errorf("invalid speaker address %q", host)
		}
	}
	return nil
}

// ── SOAP plumbing ────────────────────────────────────────────────────────

// service describes one UPnP service endpoint on the speaker.
type service struct {
	path string // control URL path
	urn  string // service type URN
}

var (
	avTransport      = service{"/MediaRenderer/AVTransport/Control", "urn:schemas-upnp-org:service:AVTransport:1"}
	renderingControl = service{"/MediaRenderer/RenderingControl/Control", "urn:schemas-upnp-org:service:RenderingControl:1"}
	groupRendering   = service{"/MediaRenderer/GroupRenderingControl/Control", "urn:schemas-upnp-org:service:GroupRenderingControl:1"}
	zoneGroupTopo    = service{"/ZoneGroupTopology/Control", "urn:schemas-upnp-org:service:ZoneGroupTopology:1"}
	contentDirectory = service{"/MediaServer/ContentDirectory/Control", "urn:schemas-upnp-org:service:ContentDirectory:1"}
)

// arg is one named SOAP argument. Order matters to UPnP, so arguments are a
// slice, not a map.
type arg struct{ name, value string }

// soapCall performs one SOAP action against a speaker and returns the raw
// response body. The response stays unparsed here; callers pick out the
// tags they need with extractTag.
func soapCall(ctx context.Context, ip string, svc service, action string, args []arg) (string, error) {
	if err := ValidateHost(ip); err != nil {
		return "", fmt.Errorf("sonos: %w", err)
	}

	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&body, `<u:%s xmlns:u="%s">`, action, svc.urn)
	for _, a := range args {
		fmt.Fprintf(&body, "<%s>%s</%s>", a.name, xmlEscape(a.value), a.name)
	}
	fmt.Fprintf(&body, `</u:%s></s:Body></s:Envelope>`, action)

	u := fmt.Sprintf("http://%s:%d%s", ip, Port, svc.path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body.Bytes()))
	if err != nil {
		return "", fmt.Errorf("sonos: build request: %w", err)
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf("%q", svc.urn+"#"+action))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sonos: %s %s: %w", ip, action, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("sonos: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		// SOAP faults carry a UPnP error code worth surfacing.
		if code := extractTag(string(raw), "errorCode"); code != "" {
			return "", fmt.Errorf("sonos: %s refused %s (UPnP error %s)", ip, action, code)
		}
		return "", fmt.Errorf("sonos: HTTP %d from %s for %s", resp.StatusCode, ip, action)
	}
	return string(raw), nil
}

// xmlEscape escapes a value for embedding in a SOAP argument.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// extractTag returns the text content of the first <tag>…</tag> in body,
// XML-unescaped. Empty when absent. Good enough for flat SOAP responses
// where full document parsing is overkill.
func extractTag(body, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(body, open)
	if i < 0 {
		return ""
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return html.UnescapeString(rest[:j])
}

// ── Device identity ──────────────────────────────────────────────────────

// Device is a speaker's identity as read from its device description.
type Device struct {
	IP    string `json:"ip"`
	UUID  string `json:"uuid"` // RINCON_… (uuid: prefix stripped)
	Room  string `json:"room"` // Sonos zone name, e.g. "Living Room"
	Model string `json:"model"`
}

// Describe fetches a speaker's device description document and returns its
// identity. Also serves as the reachability probe for "Test connection".
func Describe(ctx context.Context, ip string) (*Device, error) {
	if err := ValidateHost(ip); err != nil {
		return nil, fmt.Errorf("sonos: %w", err)
	}
	u := fmt.Sprintf("http://%s:%d/xml/device_description.xml", ip, Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sonos: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no Sonos speaker found at %s: %w", ip, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sonos at %s returned HTTP %d", ip, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("sonos: read description: %w", err)
	}
	d := ParseDescription(string(raw))
	if d.UUID == "" {
		return nil, fmt.Errorf("device at %s does not look like a Sonos speaker", ip)
	}
	d.IP = ip
	return d, nil
}

// ParseDescription pulls identity fields out of a device description
// document. Split out for testability.
func ParseDescription(body string) *Device {
	udn := extractTag(body, "UDN")
	udn = strings.TrimPrefix(udn, "uuid:")
	if !strings.HasPrefix(udn, "RINCON_") {
		udn = ""
	}
	return &Device{
		UUID:  udn,
		Room:  extractTag(body, "roomName"),
		Model: extractTag(body, "modelName"),
	}
}

// ── Transport & rendering control ────────────────────────────────────────

// Track is the now-playing metadata parsed from DIDL-Lite.
type Track struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	// ArtURI is either an absolute URL or a path relative to the speaker
	// (e.g. /getaa?...). Relative paths must be proxied by the caller.
	ArtURI string `json:"art_uri,omitempty"`
}

// State is one speaker's live playback state.
type State struct {
	TransportState string `json:"transport_state"` // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
	Playing        bool   `json:"playing"`
	Volume         int    `json:"volume"` // 0-100
	Muted          bool   `json:"muted"`
	Track          *Track `json:"track,omitempty"`
	Position       string `json:"position,omitempty"` // H:MM:SS
	Duration       string `json:"duration,omitempty"` // H:MM:SS; empty for live streams
	// QueueTrack is the 1-based position of the current track in the group
	// queue. Zero when the source isn't the queue (radio, line-in, TV).
	QueueTrack int `json:"queue_track,omitempty"`
}

// GroupState is the playback configuration that belongs to a zone group
// rather than to one speaker: shuffle, repeat, crossfade and the queue.
// Only meaningful on a coordinator, so it is fetched separately from State —
// asking every follower for it would triple the poll for no new information.
type GroupState struct {
	Shuffle   bool   `json:"shuffle"`
	Repeat    string `json:"repeat"` // off | all | one
	Crossfade bool   `json:"crossfade"`
	// QueueLength is how many tracks the group queue holds, and FromQueue
	// says whether the group is currently playing *from* that queue — a
	// group on radio still has a queue sitting behind the stream.
	QueueLength int  `json:"queue_length"`
	FromQueue   bool `json:"from_queue"`
}

const instance0 = "0"

// Play resumes/starts playback. Send to the group coordinator.
func Play(ctx context.Context, ip string) error {
	_, err := soapCall(ctx, ip, avTransport, "Play", []arg{{"InstanceID", instance0}, {"Speed", "1"}})
	return err
}

// Pause pauses playback. Radio streams don't support pause; fall back to Stop.
func Pause(ctx context.Context, ip string) error {
	if _, err := soapCall(ctx, ip, avTransport, "Pause", []arg{{"InstanceID", instance0}}); err != nil {
		_, err2 := soapCall(ctx, ip, avTransport, "Stop", []arg{{"InstanceID", instance0}})
		if err2 != nil {
			return err
		}
	}
	return nil
}

// Next skips to the next track.
func Next(ctx context.Context, ip string) error {
	_, err := soapCall(ctx, ip, avTransport, "Next", []arg{{"InstanceID", instance0}})
	return err
}

// Previous goes back one track.
func Previous(ctx context.Context, ip string) error {
	_, err := soapCall(ctx, ip, avTransport, "Previous", []arg{{"InstanceID", instance0}})
	return err
}

// GetVolume reads the speaker's own (not group) volume.
func GetVolume(ctx context.Context, ip string) (int, error) {
	body, err := soapCall(ctx, ip, renderingControl, "GetVolume",
		[]arg{{"InstanceID", instance0}, {"Channel", "Master"}})
	if err != nil {
		return 0, err
	}
	v, _ := strconv.Atoi(extractTag(body, "CurrentVolume"))
	return v, nil
}

// SetVolume sets the speaker's own volume (0-100).
func SetVolume(ctx context.Context, ip string, level int) error {
	_, err := soapCall(ctx, ip, renderingControl, "SetVolume",
		[]arg{{"InstanceID", instance0}, {"Channel", "Master"}, {"DesiredVolume", strconv.Itoa(clamp(level, 0, 100))}})
	return err
}

// GetMute reads the speaker's mute state.
func GetMute(ctx context.Context, ip string) (bool, error) {
	body, err := soapCall(ctx, ip, renderingControl, "GetMute",
		[]arg{{"InstanceID", instance0}, {"Channel", "Master"}})
	if err != nil {
		return false, err
	}
	return extractTag(body, "CurrentMute") == "1", nil
}

// SetMute mutes/unmutes the speaker.
func SetMute(ctx context.Context, ip string, muted bool) error {
	v := "0"
	if muted {
		v = "1"
	}
	_, err := soapCall(ctx, ip, renderingControl, "SetMute",
		[]arg{{"InstanceID", instance0}, {"Channel", "Master"}, {"DesiredMute", v}})
	return err
}

// SetGroupVolume sets the volume of the whole group, preserving the
// relative levels of its members. Must be sent to the group coordinator.
func SetGroupVolume(ctx context.Context, ip string, level int) error {
	_, err := soapCall(ctx, ip, groupRendering, "SetGroupVolume",
		[]arg{{"InstanceID", instance0}, {"DesiredVolume", strconv.Itoa(clamp(level, 0, 100))}})
	return err
}

// GetState gathers transport state, now-playing metadata and volume in one
// call. Partial failures degrade gracefully: an unreachable sub-request
// leaves its fields zeroed rather than failing the whole state.
func GetState(ctx context.Context, ip string) (*State, error) {
	body, err := soapCall(ctx, ip, avTransport, "GetTransportInfo", []arg{{"InstanceID", instance0}})
	if err != nil {
		return nil, err
	}
	st := &State{TransportState: extractTag(body, "CurrentTransportState")}
	st.Playing = st.TransportState == "PLAYING" || st.TransportState == "TRANSITIONING"

	if body, err := soapCall(ctx, ip, avTransport, "GetPositionInfo", []arg{{"InstanceID", instance0}}); err == nil {
		st.Position = normalizeClock(extractTag(body, "RelTime"))
		st.Duration = normalizeClock(extractTag(body, "TrackDuration"))
		st.QueueTrack, _ = strconv.Atoi(extractTag(body, "Track"))
		if meta := extractTag(body, "TrackMetaData"); meta != "" && meta != "NOT_IMPLEMENTED" {
			st.Track = ParseTrackMeta(meta)
		}
	}
	if v, err := GetVolume(ctx, ip); err == nil {
		st.Volume = v
	}
	if m, err := GetMute(ctx, ip); err == nil {
		st.Muted = m
	}
	return st, nil
}

// normalizeClock maps UPnP's "NOT_IMPLEMENTED" and zero-duration
// placeholders to empty strings the UI can treat as "no value".
func normalizeClock(s string) string {
	if s == "NOT_IMPLEMENTED" || s == "0:00:00" {
		return ""
	}
	return s
}

// didlLite is the subset of DIDL-Lite metadata we care about.
type didlLite struct {
	Items []didlItem `xml:"item"`
}

type didlItem struct {
	ID          string `xml:"id,attr"`
	Title       string `xml:"title"`
	Creator     string `xml:"creator"`
	Album       string `xml:"album"`
	AlbumArtURI string `xml:"albumArtURI"`
	Res         string `xml:"res"`
	ResMD       string `xml:"resMD"`
	Description string `xml:"description"`
}

// ParseTrackMeta parses a DIDL-Lite fragment (already XML-unescaped) into a
// Track. Returns nil when nothing useful is present.
func ParseTrackMeta(meta string) *Track {
	var d didlLite
	if err := xml.Unmarshal([]byte(meta), &d); err != nil || len(d.Items) == 0 {
		return nil
	}
	it := d.Items[0]
	t := &Track{Title: it.Title, Artist: it.Creator, Album: it.Album, ArtURI: it.AlbumArtURI}
	if t.Title == "" && t.Artist == "" && t.Album == "" {
		return nil
	}
	return t
}

// ── Grouping ─────────────────────────────────────────────────────────────

// Group is one zone group from the topology: a coordinator plus members
// (which include the coordinator itself).
type Group struct {
	CoordinatorUUID string   `json:"coordinator_uuid"`
	Members         []Member `json:"members"`
}

// Member is one speaker inside a zone group.
type Member struct {
	UUID string `json:"uuid"`
	IP   string `json:"ip"`
	Name string `json:"name"` // Sonos zone name
}

// GetTopology asks one speaker for the whole household's zone group state.
// Any speaker can answer for all of them.
func GetTopology(ctx context.Context, ip string) ([]Group, error) {
	body, err := soapCall(ctx, ip, zoneGroupTopo, "GetZoneGroupState", nil)
	if err != nil {
		return nil, err
	}
	state := extractTag(body, "ZoneGroupState")
	if state == "" {
		return nil, fmt.Errorf("sonos: %s returned empty zone group state", ip)
	}
	return ParseZoneGroupState(state)
}

// zoneGroupStateXML mirrors the ZoneGroupState document. Older firmware
// omits the wrapping <ZoneGroupState> element, so both shapes are handled
// by parsing from <ZoneGroups> down.
type zoneGroupStateXML struct {
	Groups []struct {
		Coordinator string `xml:"Coordinator,attr"`
		Members     []struct {
			UUID      string `xml:"UUID,attr"`
			Location  string `xml:"Location,attr"`
			ZoneName  string `xml:"ZoneName,attr"`
			Invisible string `xml:"Invisible,attr"`
		} `xml:"ZoneGroupMember"`
	} `xml:"ZoneGroup"`
}

// ParseZoneGroupState parses the (unescaped) ZoneGroupState document.
// Invisible members — stereo-pair satellites, Subs — are dropped: they are
// not independently controllable rooms.
func ParseZoneGroupState(state string) ([]Group, error) {
	i := strings.Index(state, "<ZoneGroups")
	if i < 0 {
		return nil, errors.New("sonos: no ZoneGroups element in topology")
	}
	// Cut to just the <ZoneGroups>…</ZoneGroups> subtree.
	sub := state[i:]
	if j := strings.Index(sub, "</ZoneGroups>"); j >= 0 {
		sub = sub[:j+len("</ZoneGroups>")]
	}
	var parsed zoneGroupStateXML
	if err := xml.Unmarshal([]byte(sub), &parsed); err != nil {
		return nil, fmt.Errorf("sonos: parse topology: %w", err)
	}
	var groups []Group
	for _, g := range parsed.Groups {
		grp := Group{CoordinatorUUID: g.Coordinator}
		for _, m := range g.Members {
			if m.Invisible == "1" {
				continue
			}
			grp.Members = append(grp.Members, Member{
				UUID: m.UUID,
				IP:   ipFromLocation(m.Location),
				Name: m.ZoneName,
			})
		}
		if len(grp.Members) > 0 {
			groups = append(groups, grp)
		}
	}
	return groups, nil
}

// ipFromLocation extracts the host from a member's Location URL
// (http://192.168.1.50:1400/xml/device_description.xml → 192.168.1.50).
func ipFromLocation(loc string) string {
	s := strings.TrimPrefix(loc, "http://")
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Join makes the speaker at ip play whatever the group led by
// coordinatorUUID is playing (i.e. joins that group).
func Join(ctx context.Context, ip, coordinatorUUID string) error {
	if !strings.HasPrefix(coordinatorUUID, "RINCON_") {
		return fmt.Errorf("sonos: %q is not a Sonos device id", coordinatorUUID)
	}
	_, err := soapCall(ctx, ip, avTransport, "SetAVTransportURI",
		[]arg{{"InstanceID", instance0}, {"CurrentURI", "x-rincon:" + coordinatorUUID}, {"CurrentURIMetaData", ""}})
	return err
}

// Leave detaches the speaker from its group, making it standalone.
func Leave(ctx context.Context, ip string) error {
	_, err := soapCall(ctx, ip, avTransport, "BecomeCoordinatorOfStandaloneGroup",
		[]arg{{"InstanceID", instance0}})
	return err
}

// ── Favorites ────────────────────────────────────────────────────────────

// Favorite is one entry from the speaker's "My Sonos" favorites list.
// URI + Metadata are opaque round-trip values handed back to PlayFavorite.
type Favorite struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	ArtURI   string `json:"art_uri,omitempty"`
	URI      string `json:"uri"`
	Metadata string `json:"metadata,omitempty"`
	Service  string `json:"service,omitempty"` // human label, e.g. "TuneIn"
}

// ListFavorites browses the household's Sonos favorites ("My Sonos").
// Favorites are shared across the household, so any speaker can answer.
func ListFavorites(ctx context.Context, ip string) ([]Favorite, error) {
	body, err := soapCall(ctx, ip, contentDirectory, "Browse", []arg{
		{"ObjectID", "FV:2"},
		{"BrowseFlag", "BrowseDirectChildren"},
		{"Filter", "*"},
		{"StartingIndex", "0"},
		{"RequestedCount", "100"},
		{"SortCriteria", ""},
	})
	if err != nil {
		return nil, err
	}
	result := extractTag(body, "Result")
	if result == "" {
		return []Favorite{}, nil
	}
	return ParseFavorites(result)
}

// ParseFavorites parses the (unescaped) DIDL-Lite favorites listing.
func ParseFavorites(result string) ([]Favorite, error) {
	var d didlLite
	if err := xml.Unmarshal([]byte(result), &d); err != nil {
		return nil, fmt.Errorf("sonos: parse favorites: %w", err)
	}
	favs := make([]Favorite, 0, len(d.Items))
	for _, it := range d.Items {
		if it.Res == "" {
			continue
		}
		favs = append(favs, Favorite{
			ID:       it.ID,
			Title:    it.Title,
			ArtURI:   it.AlbumArtURI,
			URI:      it.Res,
			Metadata: it.ResMD,
			Service:  it.Description,
		})
	}
	return favs, nil
}

// PlayFavorite starts a favorite on the group led by the speaker at ip
// (which must be a coordinator). Container favorites (playlists, albums)
// are loaded into the queue; streams (radio) are set as the transport URI
// directly. speakerUUID is the coordinator's RINCON id, needed to address
// its queue.
func PlayFavorite(ctx context.Context, ip, speakerUUID string, fav Favorite) error {
	if fav.URI == "" {
		return errors.New("sonos: favorite has no URI")
	}
	if isContainerURI(fav.URI) {
		if !strings.HasPrefix(speakerUUID, "RINCON_") {
			return fmt.Errorf("sonos: %q is not a Sonos device id", speakerUUID)
		}
		if _, err := soapCall(ctx, ip, avTransport, "RemoveAllTracksFromQueue",
			[]arg{{"InstanceID", instance0}}); err != nil {
			return err
		}
		if _, err := soapCall(ctx, ip, avTransport, "AddURIToQueue", []arg{
			{"InstanceID", instance0},
			{"EnqueuedURI", fav.URI},
			{"EnqueuedURIMetaData", fav.Metadata},
			{"DesiredFirstTrackNumberEnqueued", "0"},
			{"EnqueueAsNext", "0"},
		}); err != nil {
			return err
		}
		if _, err := soapCall(ctx, ip, avTransport, "SetAVTransportURI", []arg{
			{"InstanceID", instance0},
			{"CurrentURI", "x-rincon-queue:" + speakerUUID + "#0"},
			{"CurrentURIMetaData", ""},
		}); err != nil {
			return err
		}
	} else {
		if _, err := soapCall(ctx, ip, avTransport, "SetAVTransportURI", []arg{
			{"InstanceID", instance0},
			{"CurrentURI", fav.URI},
			{"CurrentURIMetaData", fav.Metadata},
		}); err != nil {
			return err
		}
	}
	return Play(ctx, ip)
}

// isContainerURI reports whether a favorite URI refers to a container
// (playlist/album) that must go through the queue rather than being set as
// the transport URI directly.
func isContainerURI(uri string) bool {
	return strings.HasPrefix(uri, "x-rincon-cpcontainer:") ||
		strings.HasPrefix(uri, "file:///jffs/settings/savedqueues.rsq") ||
		strings.HasPrefix(uri, "x-rincon-playlist:")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
