package airplay

// Track info for the receiver's display.
//
// RAOP carries metadata as DAAP — iTunes' own tag format, four-character tag
// plus a big-endian length plus the value, nested one container deep. It is
// what fills in RoPieee's display and what a receiver logs as "now playing",
// and without it a listener sees the sender's name and nothing else.
//
// Artwork is deliberately not sent. It would mean fetching the image
// server-side and pushing a JPEG down the control channel per track; the
// media layer's ArtURI may be relative to a speaker that is not this one (the
// Sonos rule, carried in the Metadata doc comment), so resolving it here would
// mean this package growing an HTTP client to guess at another bridge's
// addressing. The text is the part that earns its keep.

import "encoding/binary"

// daapContentType is what SET_PARAMETER declares for a metadata body.
const daapContentType = "application/x-dmap-tagged"

// daapTags are the three fields a receiver's display actually shows.
const (
	tagListing = "mlit" // the item container everything else nests in
	tagName    = "minm" // track title
	tagArtist  = "asar"
	tagAlbum   = "asal"
)

// daapMetadata encodes a track as a DAAP item. Empty fields are omitted rather
// than sent blank: a receiver showing an empty artist line looks broken, one
// showing no artist line looks like a stream with no artist.
func daapMetadata(title, artist, album string) []byte {
	var item []byte
	item = appendDAAP(item, tagName, title)
	item = appendDAAP(item, tagArtist, artist)
	item = appendDAAP(item, tagAlbum, album)
	if len(item) == 0 {
		return nil
	}
	return appendDAAPRaw(nil, tagListing, item)
}

func appendDAAP(dst []byte, tag, value string) []byte {
	if value == "" {
		return dst
	}
	return appendDAAPRaw(dst, tag, []byte(value))
}

func appendDAAPRaw(dst []byte, tag string, value []byte) []byte {
	dst = append(dst, tag...)
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(value)))
	return append(dst, value...)
}
