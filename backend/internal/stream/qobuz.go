package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"

	"homehub/internal/media"
	"homehub/internal/qobuz"
)

// Qobuz decodes Qobuz by fetching each track's signed FLAC URL and decoding it
// to interleaved PCM at the track's own rate and word length.
//
// This is the decoder the rest of the system was waiting for, and it is worth
// being precise about what "lossless" means here, because the phrase gets used
// loosely. FLAC decoding is exact: the samples that come out are bit-for-bit
// the samples that went into the encoder. So the audio HomeHub re-serves is the
// master Qobuz sold, and the only way this path could damage it is by
// converting — resampling to a house rate, or squeezing 24-bit words into 16.
// It does neither. The stream declares whatever the file turned out to be and
// the WAV header downstream describes exactly that.
//
// The shape differs from Librespot in one way that matters. librespot is a
// Connect receiver: Spotify tells it what to play and it plays a queue. Qobuz
// hands out one file at a time, so HomeHub owns the sequencing — an album is a
// list of tracks this decoder walks, opening each in turn. That is why Open
// takes a URI that may name an album or a playlist and why the pump below has
// a track index in it.
type Qobuz struct {
	cfg QobuzConfig

	mu      sync.Mutex
	running *flacPump
}

// Catalog is the slice of the Qobuz client this decoder needs. Narrow on
// purpose: the decoder does not search, and taking the whole client would make
// it untestable without one.
type Catalog interface {
	// Tracks expands a playable URI into the tracks to play, in order.
	Tracks(ctx context.Context, uri string) ([]qobuz.Item, error)
	// FileURL returns a signed URL for one track at the best format allowed.
	FileURL(ctx context.Context, trackID string, want qobuz.FormatID) (*qobuz.File, error)
	// MaxFormat is the best format this account may request.
	MaxFormat() qobuz.FormatID
}

// QobuzConfig configures the decoder.
type QobuzConfig struct {
	// Catalog resolves URIs and signs file requests. Required.
	Catalog Catalog
	// HTTP fetches the audio files. Defaults to a client with no overall
	// timeout: a 20-minute 24-bit/192 kHz track is a long download, and a
	// deadline that made sense for an API call would cut it off mid-piece.
	HTTP *http.Client
	Logf func(format string, args ...any)
}

// NewQobuz creates a decoder.
func NewQobuz(cfg QobuzConfig) *Qobuz {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	return &Qobuz{cfg: cfg}
}

func (q *Qobuz) logf(format string, args ...any) {
	if q.cfg.Logf != nil {
		q.cfg.Logf(format, args...)
	}
}

// Available reports whether decoding could work. Unlike librespot there is no
// binary to find: everything this decoder needs is in-process, so the only
// question is whether the account is wired up.
func (q *Qobuz) Available() media.Availability {
	if q.cfg.Catalog == nil {
		return media.Availability{Reason: "Qobuz isn't set up on this server"}
	}
	return media.Availability{OK: true, Configured: true}
}

// Open begins decoding uri, which may name a track, an album or a playlist.
//
// Only one decode runs at a time, matching Librespot: a second would be a
// second stream competing for the same speakers, and replacing the first is
// what the user asked for by starting something new.
func (q *Qobuz) Open(ctx context.Context, uri string) (*media.Stream, error) {
	if av := q.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}

	tracks, err := q.cfg.Catalog.Tracks(ctx, uri)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, errors.New("stream: that Qobuz link has nothing to play")
	}

	// The first track is opened here rather than lazily, for two reasons: an
	// unplayable link should fail the tap rather than produce a silent
	// stream, and the format the whole stream will be declared in is not
	// known until a real file has been read.
	procCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p := &flacPump{
		ctx: procCtx, cancel: cancel,
		catalog: q.cfg.Catalog, http: q.cfg.HTTP,
		tracks: tracks, want: q.cfg.Catalog.MaxFormat(),
		logf: q.logf,
	}
	if err := p.openCurrent(); err != nil {
		cancel()
		return nil, err
	}

	q.mu.Lock()
	if q.running != nil {
		q.logf("stream: replacing the running Qobuz decoder")
		_ = q.running.Close()
	}
	q.running = p
	q.mu.Unlock()

	first := tracks[0]
	q.logf("stream: Qobuz decoding %q at %s", first.Name, p.format.Label())

	format := p.format
	return &media.Stream{
		Body:        p,
		ContentType: ContentTypeWAV,
		PCM:         &format,
		Meta: media.Metadata{
			Title:       first.Name,
			Artist:      first.Sub,
			ArtURI:      first.ArtURL,
			ContentType: ContentTypeWAV,
			Live:        true,
		},
	}, nil
}

// Close stops any running decode.
func (q *Qobuz) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running == nil {
		return nil
	}
	err := q.running.Close()
	q.running = nil
	return err
}

// flacPump walks a track list, decoding each file to interleaved PCM.
//
// It is an io.Reader so the Host can treat it exactly like librespot's stdout.
// The difference is invisible from outside: where librespot hands over a
// continuous pipe, this one stitches files together, and the seam is where the
// interesting rule lives — see next.
type flacPump struct {
	ctx     context.Context
	cancel  context.CancelFunc
	catalog Catalog
	http    *http.Client
	logf    func(string, ...any)

	tracks []qobuz.Item
	idx    int
	want   qobuz.FormatID

	// format is what the whole stream is declared as: the first track's.
	format media.PCMFormat

	body io.ReadCloser
	dec  *flac.Stream
	// buf holds one decoded frame and off is how much of it has been read.
	// Kept as a buffer plus an offset rather than a re-sliced slice:
	// advancing the slice header would leave each frame decoding into
	// whatever capacity the last read happened to leave behind, which turns
	// a fixed allocation into one per frame for the life of the stream.
	buf []byte
	off int
	// openedFormat is the format of the file most recently opened. It is
	// compared against format — the stream's declared one — before the
	// track is allowed to play, which is what stops an album changing
	// format mid-stream.
	openedFormat media.PCMFormat

	closeOnce sync.Once
}

// Read implements io.Reader, decoding as much as is needed to fill out.
func (p *flacPump) Read(out []byte) (int, error) {
	for p.off >= len(p.buf) {
		if err := p.ctx.Err(); err != nil {
			return 0, err
		}
		f, err := p.dec.ParseNext()
		if err == nil {
			p.buf = appendInterleaved(p.buf[:0], f.Subframes, p.format.BitDepth)
			p.off = 0
			continue
		}
		if !errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("stream: decoding %q: %w", p.currentName(), err)
		}
		if err := p.next(); err != nil {
			return 0, err
		}
	}
	n := copy(out, p.buf[p.off:])
	p.off += n
	return n, nil
}

// next advances to the following track, or reports io.EOF when the stream is
// finished.
//
// This is where "never downsample" is enforced at the seam. A WAV stream
// declares one format in its header and carries it to the end, so a track in a
// different format cannot be appended to this one without being converted.
// HomeHub does not convert, so the stream ends here instead and the next play
// starts a fresh one at the new format. Ending a few tracks early is a visible,
// explicable thing; silently resampling the rest of an album is not.
func (p *flacPump) next() error {
	p.closeBody()
	for p.idx+1 < len(p.tracks) {
		p.idx++
		if err := p.openCurrent(); err != nil {
			// One unplayable track in an album should not end the album —
			// a purchase-only bonus track is common — so it is logged and
			// skipped rather than surfaced as a failure mid-playback.
			p.logf("stream: skipping %q: %v", p.currentName(), err)
			continue
		}
		if p.format == p.openedFormat {
			return nil
		}
		p.logf("stream: stopping before %q — it is %s and this stream is %s; HomeHub won't convert it",
			p.currentName(), p.openedFormat.Label(), p.format.Label())
		p.closeBody()
		return io.EOF
	}
	return io.EOF
}

// openCurrent fetches and opens the track at idx.
func (p *flacPump) openCurrent() error {
	track := p.tracks[p.idx]
	file, err := p.catalog.FileURL(p.ctx, track.ID, p.want)
	if err != nil {
		return err
	}
	if !file.Lossless() {
		// The account is entitled to less than FLAC, or this track only
		// exists compressed. Either way the point of this provider is gone,
		// and saying so beats quietly playing an MP3 through a path
		// advertised as lossless.
		return fmt.Errorf("stream: Qobuz offered %s for %q, not FLAC",
			file.FormatID.Label(), track.Name)
	}

	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, file.URL, nil)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("stream: fetching %q: %w", track.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("stream: fetching %q: %s", track.Name, resp.Status)
	}

	dec, err := flac.New(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return fmt.Errorf("stream: %q isn't FLAC HomeHub can read: %w", track.Name, err)
	}

	// The file's own StreamInfo is believed over the catalogue's metadata
	// and over what was requested. Those are a listing and an intention; this
	// is the audio.
	got := media.PCMFormat{
		SampleRate:   int(dec.Info.SampleRate),
		BitDepth:     int(dec.Info.BitsPerSample),
		Channels:     int(dec.Info.NChannels),
		LittleEndian: true,
	}
	if got.BitDepth != 16 && got.BitDepth != 24 {
		// 8, 12, 20 and 32-bit FLAC all exist. None appears in Qobuz's
		// catalogue, and packing them would mean choosing a conversion.
		_ = resp.Body.Close()
		return fmt.Errorf("stream: %q is %d-bit FLAC, which HomeHub can't carry without converting it",
			track.Name, got.BitDepth)
	}

	p.body, p.dec, p.openedFormat = resp.Body, dec, got
	if p.format == (media.PCMFormat{}) {
		p.format = got
	}
	return nil
}

func (p *flacPump) currentName() string {
	if p.idx < len(p.tracks) {
		return p.tracks[p.idx].Name
	}
	return "this track"
}

func (p *flacPump) closeBody() {
	if p.body != nil {
		_ = p.body.Close()
		p.body, p.dec = nil, nil
	}
}

// Close stops the pump and releases the current file. Safe to call repeatedly.
func (p *flacPump) Close() error {
	p.closeOnce.Do(func() {
		p.cancel()
		p.closeBody()
	})
	return nil
}

// appendInterleaved packs decoded FLAC subframes into interleaved
// little-endian PCM at depth bits per sample.
//
// FLAC decodes to one signed int32 slice per channel; every wire format that
// carries PCM wants them interleaved. The packing is a straight two's
// complement truncation to the file's own word length — 16-bit samples get two
// bytes, 24-bit samples get three — which is a re-packing rather than a
// conversion: no sample's value changes, so this stays bit-exact.
func appendInterleaved(dst []byte, subframes []*frame.Subframe, depth int) []byte {
	if len(subframes) == 0 {
		return dst
	}
	n := len(subframes[0].Samples)
	bytesPer := depth / 8
	dst = growBytes(dst, n*len(subframes)*bytesPer)
	for i := 0; i < n; i++ {
		for _, sf := range subframes {
			if i >= len(sf.Samples) {
				continue
			}
			v := uint32(sf.Samples[i])
			for b := 0; b < bytesPer; b++ {
				dst = append(dst, byte(v>>(8*b)))
			}
		}
	}
	return dst
}

// growBytes makes room for n more bytes without changing length.
func growBytes(dst []byte, n int) []byte {
	if cap(dst)-len(dst) >= n {
		return dst
	}
	grown := make([]byte, len(dst), len(dst)+n)
	copy(grown, dst)
	return grown
}
