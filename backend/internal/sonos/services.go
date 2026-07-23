package sonos

// Music-service playback: turning a streaming-service item (e.g. a Spotify
// track found via the Spotify Web API) into something a speaker will play.
//
// The speaker streams the service itself using the account linked to the
// Sonos household — we only have to hand it a service URI plus DIDL metadata
// whose <desc> token names that account. The URI/metadata conventions below
// follow the widely-deployed node-sonos / node-sonos-http-api mappings.
// The service id (sid) and account serial (sn) differ per household and
// region, so they are always read from the speaker, never hardcoded.

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var musicServices = service{"/MusicServices/Control", "urn:schemas-upnp-org:service:MusicServices:1"}

// ServiceAccount identifies one streaming service linked to the household,
// as needed to build playable URIs.
type ServiceAccount struct {
	Name        string // e.g. "Spotify"
	SID         int    // service id used in URIs (?sid=)
	SerialNum   string // account serial used in URIs (?sn=)
	ServiceType int    // sid<<8|7 — used in the DIDL desc token
}

// GetServiceAccount resolves the household's account for the named service
// (case-insensitive) by asking the speaker which services exist and which
// accounts are linked. Returns an error naming the service when it isn't
// linked to the household.
func GetServiceAccount(ctx context.Context, ip, serviceName string) (*ServiceAccount, error) {
	body, err := soapCall(ctx, ip, musicServices, "ListAvailableServices", nil)
	if err != nil {
		return nil, err
	}
	list := extractTag(body, "AvailableServiceDescriptorList")
	sid, err := parseServiceID(list, serviceName)
	if err != nil {
		return nil, err
	}
	acct := &ServiceAccount{
		Name:        serviceName,
		SID:         sid,
		ServiceType: sid<<8 | 7,
		SerialNum:   "1", // single-account default when /status/accounts is unavailable
	}
	if sn := fetchAccountSerial(ctx, ip, acct.ServiceType); sn != "" {
		acct.SerialNum = sn
	}
	return acct, nil
}

// serviceListXML mirrors the AvailableServiceDescriptorList document.
type serviceListXML struct {
	Services []struct {
		ID   string `xml:"Id,attr"`
		Name string `xml:"Name,attr"`
	} `xml:"Service"`
}

// parseServiceID finds the named service's id in the (unescaped) descriptor
// list. Split out for testability.
func parseServiceID(list, serviceName string) (int, error) {
	var parsed serviceListXML
	if err := xml.Unmarshal([]byte(list), &parsed); err != nil {
		return 0, fmt.Errorf("sonos: parse service list: %w", err)
	}
	for _, s := range parsed.Services {
		if strings.EqualFold(s.Name, serviceName) {
			id, err := strconv.Atoi(s.ID)
			if err != nil {
				return 0, fmt.Errorf("sonos: service %q has non-numeric id %q", serviceName, s.ID)
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("sonos: %s is not linked to this Sonos household — add it once in the Sonos app", serviceName)
}

// accountsXML mirrors the /status/accounts document.
type accountsXML struct {
	Accounts []struct {
		Type      string `xml:"Type,attr"`
		SerialNum string `xml:"SerialNum,attr"`
		Deleted   string `xml:"Deleted,attr"`
	} `xml:"Accounts>Account"`
}

// fetchAccountSerial reads the account serial for a service type from the
// speaker's /status/accounts page. Best-effort: newer firmware sometimes
// hides this page, in which case the "1" default usually holds.
func fetchAccountSerial(ctx context.Context, ip string, serviceType int) string {
	if ValidateHost(ip) != nil {
		return ""
	}
	u := fmt.Sprintf("http://%s:%d/status/accounts", ip, Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	return parseAccountSerial(string(raw), serviceType)
}

// parseAccountSerial picks the serial of the first non-deleted account with
// the given type. Split out for testability.
func parseAccountSerial(body string, serviceType int) string {
	var parsed accountsXML
	if err := xml.Unmarshal([]byte(body), &parsed); err != nil {
		return ""
	}
	want := strconv.Itoa(serviceType)
	for _, a := range parsed.Accounts {
		if a.Type == want && a.Deleted != "1" {
			return a.SerialNum
		}
	}
	return ""
}

// SpotifyItem maps a canonical Spotify URI (spotify:track:… /
// spotify:album:… / spotify:playlist:…) to a Sonos-playable URI + DIDL
// metadata for the given account. Title is embedded in the metadata so the
// queue shows a name immediately.
func SpotifyItem(spotifyURI, title string, acct *ServiceAccount) (uri, metadata string, err error) {
	enc := url.QueryEscape(spotifyURI) // Sonos wants %3a for the colons
	suffix := fmt.Sprintf("?sid=%d&flags=8224&sn=%s", acct.SID, acct.SerialNum)

	var itemID, class string
	switch {
	case strings.HasPrefix(spotifyURI, "spotify:track:"):
		itemID = "00032020" + enc
		class = "object.item.audioItem.musicTrack"
		uri = "x-sonos-spotify:" + enc + suffix
	case strings.HasPrefix(spotifyURI, "spotify:album:"):
		itemID = "0004206c" + enc
		class = "object.container.album.musicAlbum"
		uri = "x-rincon-cpcontainer:" + itemID + suffix
	case strings.HasPrefix(spotifyURI, "spotify:playlist:"):
		itemID = "0006206c" + enc
		class = "object.container.playlistContainer"
		uri = "x-rincon-cpcontainer:" + itemID + suffix
	default:
		return "", "", fmt.Errorf("sonos: unsupported Spotify URI %q", spotifyURI)
	}
	return uri, serviceItemMetadata(itemID, title, class, acct.ServiceType), nil
}

// serviceItemMetadata builds the DIDL-Lite fragment for a music-service
// item. The cdudn <desc> token is what tells the speaker which linked
// account to stream with.
func serviceItemMetadata(itemID, title, class string, serviceType int) string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="` + xmlEscape(itemID) + `" restricted="true">` +
		`<dc:title>` + xmlEscape(title) + `</dc:title>` +
		`<upnp:class>` + class + `</upnp:class>` +
		`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">` +
		fmt.Sprintf("SA_RINCON%d_X_#Svc%d-0-Token", serviceType, serviceType) +
		`</desc></item></DIDL-Lite>`
}

// PlayServiceItem replaces the group queue with the item and starts
// playback. Send to the group coordinator; speakerUUID is its RINCON id
// (needed to address its queue). Both tracks and containers go through the
// queue — that is the path Sonos itself uses for on-demand service content.
func PlayServiceItem(ctx context.Context, ip, speakerUUID, uri, metadata string) error {
	if !strings.HasPrefix(speakerUUID, "RINCON_") {
		return fmt.Errorf("sonos: %q is not a Sonos device id", speakerUUID)
	}
	if _, err := soapCall(ctx, ip, avTransport, "RemoveAllTracksFromQueue",
		[]arg{{"InstanceID", instance0}}); err != nil {
		return err
	}
	if _, err := soapCall(ctx, ip, avTransport, "AddURIToQueue", []arg{
		{"InstanceID", instance0},
		{"EnqueuedURI", uri},
		{"EnqueuedURIMetaData", metadata},
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
	return Play(ctx, ip)
}
