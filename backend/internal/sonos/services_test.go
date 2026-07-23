package sonos

import (
	"strings"
	"testing"
)

func TestParseServiceID(t *testing.T) {
	list := `<Services SchemaVersion="1">` +
		`<Service Id="254" Name="TuneIn" Version="1.1"/>` +
		`<Service Id="12" Name="Spotify" Version="1.1"/>` +
		`<Service Id="31" Name="Qobuz" Version="1.1"/>` +
		`</Services>`
	id, err := parseServiceID(list, "spotify")
	if err != nil {
		t.Fatalf("parseServiceID: %v", err)
	}
	if id != 12 {
		t.Errorf("sid = %d, want 12", id)
	}
	if _, err := parseServiceID(list, "Deezer"); err == nil {
		t.Error("unlinked service should error")
	} else if !strings.Contains(err.Error(), "Deezer") {
		t.Errorf("error should name the service: %v", err)
	}
}

func TestParseAccountSerial(t *testing.T) {
	body := `<ZPSupportInfo><Accounts LastUpdateDevice="RINCON_X" Version="8" NextSerialNum="4">` +
		`<Account Type="65031" SerialNum="0" Deleted="0"><UN></UN></Account>` +
		`<Account Type="3079" SerialNum="2" Deleted="1"><UN>old</UN></Account>` +
		`<Account Type="3079" SerialNum="3" Deleted="0"><UN>current</UN></Account>` +
		`</Accounts></ZPSupportInfo>`
	if sn := parseAccountSerial(body, 3079); sn != "3" {
		t.Errorf("serial = %q, want 3 (deleted account skipped)", sn)
	}
	if sn := parseAccountSerial(body, 9991); sn != "" {
		t.Errorf("unknown type should return empty, got %q", sn)
	}
	if sn := parseAccountSerial("not xml", 3079); sn != "" {
		t.Errorf("garbage should return empty, got %q", sn)
	}
}

func TestSpotifyItem(t *testing.T) {
	acct := &ServiceAccount{Name: "Spotify", SID: 12, SerialNum: "3", ServiceType: 3079}

	uri, meta, err := SpotifyItem("spotify:track:4uLU6hMCjMI75M1A2tKUQC", "Never Gonna Give You Up", acct)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	if uri != "x-sonos-spotify:spotify%3Atrack%3A4uLU6hMCjMI75M1A2tKUQC?sid=12&flags=8224&sn=3" {
		t.Errorf("track uri = %q", uri)
	}
	if !strings.Contains(meta, `id="00032020spotify%3Atrack%3A4uLU6hMCjMI75M1A2tKUQC"`) {
		t.Errorf("track item id missing: %q", meta)
	}
	if !strings.Contains(meta, "object.item.audioItem.musicTrack") {
		t.Errorf("track class missing: %q", meta)
	}
	if !strings.Contains(meta, "SA_RINCON3079_X_#Svc3079-0-Token") {
		t.Errorf("account token missing: %q", meta)
	}

	uri, meta, err = SpotifyItem("spotify:album:abc", "OK Computer", acct)
	if err != nil {
		t.Fatalf("album: %v", err)
	}
	if !strings.HasPrefix(uri, "x-rincon-cpcontainer:0004206cspotify%3Aalbum%3Aabc") {
		t.Errorf("album uri = %q", uri)
	}
	if !strings.Contains(meta, "object.container.album.musicAlbum") {
		t.Errorf("album class missing: %q", meta)
	}

	uri, _, err = SpotifyItem("spotify:playlist:xyz", "Mix", acct)
	if err != nil {
		t.Fatalf("playlist: %v", err)
	}
	if !strings.HasPrefix(uri, "x-rincon-cpcontainer:0006206cspotify%3Aplaylist%3Axyz") {
		t.Errorf("playlist uri = %q", uri)
	}
	// Container URIs must take the queue path in PlayFavorite-style checks.
	if !isContainerURI(uri) {
		t.Error("playlist uri should be recognised as a container")
	}

	if _, _, err := SpotifyItem("spotify:artist:nope", "x", acct); err == nil {
		t.Error("unsupported kind should error")
	}
	// Titles with XML metacharacters must be escaped into the metadata.
	_, meta, err = SpotifyItem("spotify:track:t", `Bed & Breakfast <3`, acct)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(meta, "Bed &amp; Breakfast &lt;3") {
		t.Errorf("title not escaped: %q", meta)
	}
}
