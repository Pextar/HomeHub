package airplay

// A cast is one decode driving several receivers.
//
// This is the piece the media layer actually holds: it takes the same
// *media.Stream the HTTP stream route takes, and instead of publishing a URL
// for speakers to fetch, it pushes the audio to every receiver itself, from
// one reader and one clock. One decode, many sinks — which is what makes the
// sync worth having, and what makes a receiver joining late impossible: they
// all start from the same packet.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"homehub/internal/media"
)

// packetDuration is how much wall-clock time one packet of audio represents.
// Just under 8 ms, and the interval the pump paces itself to.
const packetDuration = time.Duration(FramesPerPacket) * time.Second / SampleRate

// paceSlack is how far ahead of the clock the pump is allowed to run before it
// waits. Some slack is wanted — the source delivers in chunks, and correcting
// every fraction of a millisecond would cost more in sleeps than it buys — but
// unbounded, a source that hands over a burst would overrun the receivers'
// buffers and be dropped.
const paceSlack = 100 * time.Millisecond

// Caster runs AirPlay casts and remembers the live one, so the endpoints in it
// can still be controlled while it plays.
//
// One cast at a time. Two would mean two decodes of two different things going
// to overlapping sets of receivers, and the decoder upstream is itself
// single-session (see internal/stream) — so starting a second replaces the
// first, which is what the user did.
type Caster struct {
	logf func(format string, args ...any)

	mu   sync.Mutex
	live *cast
}

// NewCaster creates a Caster. Nothing runs until Cast is called.
func NewCaster(logf func(format string, args ...any)) *Caster {
	return &Caster{logf: logf}
}

func (c *Caster) log(format string, args ...any) {
	if c.logf != nil {
		c.logf(format, args...)
	}
}

// cast is one running fan-out.
type cast struct {
	sessions map[string]*Session // keyed by the endpoint id the caller gave
	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup

	mu     sync.Mutex
	paused bool
}

// Cast opens a session per destination and starts pushing s to all of them.
//
// Every destination must succeed. A partial cast is music coming out of some
// of the room with no error to explain the silence in the rest, which is the
// same judgement playStream makes in the media layer and for the same reason.
func (c *Caster) Cast(ctx context.Context, s *media.Stream, dests []media.AirPlayDest) (func(), error) {
	if s == nil || s.Body == nil {
		return nil, errors.New("airplay: nothing to cast")
	}
	if len(dests) == 0 {
		return nil, errors.New("airplay: no receivers to cast to")
	}
	if !s.PCM.Matches(media.CDQuality) {
		// The pump reads raw interleaved S16LE at CD rate and packs it
		// straight into RTP. Anything else — a container, another rate —
		// would need a decoder or a resampler this package does not have,
		// and guessing would put noise through someone's speakers at
		// whatever volume they left them on.
		return nil, fmt.Errorf(
			"airplay: this source isn't the 44.1 kHz 16-bit PCM AirPlay carries (%s)",
			s.ContentType)
	}

	// The running cast is torn down *before* the new sessions are opened,
	// not after. A receiver holds one sender at a time and answers a second
	// with "busy" — so replacing a cast that includes the same receiver would
	// fail against a session this process is itself still holding. The cost
	// is that a failure below leaves the previous cast stopped, which is the
	// right way round: the user asked for something else to play.
	c.mu.Lock()
	previous := c.live
	c.live = nil
	c.mu.Unlock()
	if previous != nil {
		c.log("airplay: replacing the running cast")
		previous.stop()
	}

	sessions, err := c.openAll(ctx, dests)
	if err != nil {
		return nil, err
	}

	run := &cast{sessions: sessions, done: make(chan struct{})}
	c.mu.Lock()
	c.live = run
	c.mu.Unlock()

	// Metadata is best-effort and off the critical path: a receiver that
	// refuses it still plays the audio.
	for _, sess := range sessions {
		if err := sess.SetMetadata(ctx, s.Meta.Title, s.Meta.Artist, s.Meta.Album); err != nil {
			c.log("airplay: %s took no metadata: %v", sess.Device().Name, err)
		}
	}

	run.wg.Add(2)
	go c.pump(run, s)
	go c.syncLoop(run)

	c.log("airplay: casting to %d receiver(s)", len(sessions))
	return run.stop, nil
}

// openAll negotiates every session, concurrently, and unwinds them all if any
// one of them refuses.
func (c *Caster) openAll(ctx context.Context, dests []media.AirPlayDest) (map[string]*Session, error) {
	type result struct {
		id   string
		sess *Session
		err  error
	}
	results := make([]result, len(dests))
	var wg sync.WaitGroup
	for i, d := range dests {
		wg.Add(1)
		go func(i int, d media.AirPlayDest) {
			defer wg.Done()
			dev := Device{
				Name: d.Name, IP: d.Host, Port: d.Port, ID: d.ID,
				Codecs:     codecsFrom(d),
				Encryption: ciphersFrom(d),
				Audio:      Audio{SampleRate: SampleRate, BitDepth: BitsPerSample, Channels: Channels},
				Metadata:   d.Metadata,
			}
			sess, err := Open(ctx, dev, Options{Volume: d.Volume, Logf: c.logf})
			results[i] = result{id: d.ID, sess: sess, err: err}
		}(i, d)
	}
	wg.Wait()

	out := make(map[string]*Session, len(dests))
	var failure error
	for _, r := range results {
		if r.err != nil {
			if failure == nil {
				failure = r.err
			}
			continue
		}
		out[r.id] = r.sess
	}
	if failure != nil {
		for _, sess := range out {
			sess.Close()
		}
		return nil, failure
	}
	return out, nil
}

// codecsFrom and ciphersFrom translate the media layer's flat description of a
// receiver back into this package's lists. The media layer carries booleans
// because it must not know AirPlay's numbering; this is where the numbering
// lives.
func codecsFrom(d media.AirPlayDest) []Codec {
	var out []Codec
	if d.PCM {
		out = append(out, CodecPCM)
	}
	if d.ALAC {
		out = append(out, CodecALAC)
	}
	if len(out) == 0 {
		out = []Codec{CodecALAC} // every RAOP receiver takes ALAC
	}
	return out
}

func ciphersFrom(d media.AirPlayDest) []Encryption {
	if d.NeedsEncryption {
		return []Encryption{EncryptionRSA}
	}
	return []Encryption{EncryptionNone, EncryptionRSA}
}

// pump reads the decoder a packet at a time and hands each packet to every
// session, paced to the wall clock.
//
// Sequential across receivers on purpose: the sends are three UDP writes of a
// kilobyte and a half, and doing them in order from one goroutine is both
// faster than scheduling three and what keeps their timestamps identical.
func (c *Caster) pump(run *cast, s *media.Stream) {
	defer run.wg.Done()
	defer func() { _ = s.Body.Close() }()

	buf := make([]byte, PacketBytes)
	start := time.Now()
	var packets int64

	for {
		select {
		case <-run.done:
			return
		default:
		}

		n, err := io.ReadFull(s.Body, buf)
		if n > 0 {
			// A short read at the end of a stream is still audio; anything
			// not on a frame boundary is not, and is dropped rather than
			// sent as a fractional sample.
			n -= n % BytesPerFrame
		}
		if n > 0 && !run.isPaused() {
			for _, sess := range run.sessions {
				if err := sess.Send(buf[:n]); err != nil {
					c.log("airplay: %v", err)
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				c.log("airplay: the decoder stopped: %v", err)
			}
			run.stop()
			return
		}

		packets++
		if ahead := time.Until(start.Add(time.Duration(packets) * packetDuration)); ahead > paceSlack {
			select {
			case <-time.After(ahead - paceSlack):
			case <-run.done:
				return
			}
		}
	}
}

// syncLoop keeps every receiver's clock aligned for as long as the cast runs.
func (c *Caster) syncLoop(run *cast) {
	defer run.wg.Done()
	for _, sess := range run.sessions {
		if err := sess.Sync(true); err != nil {
			c.log("airplay: first sync to %s: %v", sess.Device().Name, err)
		}
	}
	t := time.NewTicker(syncInterval)
	defer t.Stop()
	for {
		select {
		case <-run.done:
			return
		case <-t.C:
			if run.isPaused() {
				continue
			}
			for _, sess := range run.sessions {
				_ = sess.Sync(false)
			}
		}
	}
}

func (r *cast) isPaused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paused
}

func (r *cast) setPaused(p bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = p
}

// stop ends the cast and releases every session. Safe to call repeatedly, and
// safe to call from the pump itself, which is how a decoder that ends closes
// the thing that was reading it.
func (r *cast) stop() {
	r.stopOnce.Do(func() {
		close(r.done)
		for _, sess := range r.sessions {
			sess.Close()
		}
	})
}

// Live returns a control surface for an endpoint that a cast is currently
// driving. An AirPlay receiver has no state of its own to read — it is a sink,
// and what it is playing is whatever HomeHub is sending it — so this is the
// only way an endpoint adapter can answer a volume change or a pause.
func (c *Caster) Live(id string) (media.AirPlayControl, bool) {
	c.mu.Lock()
	run := c.live
	c.mu.Unlock()
	if run == nil {
		return nil, false
	}
	select {
	case <-run.done:
		return nil, false // finished but not yet cleared
	default:
	}
	sess, ok := run.sessions[id]
	if !ok {
		return nil, false
	}
	return &control{run: run, sess: sess}, true
}

// Close stops any running cast. Called at shutdown.
func (c *Caster) Close() {
	c.mu.Lock()
	run := c.live
	c.live = nil
	c.mu.Unlock()
	if run != nil {
		run.stop()
	}
}

// control is one endpoint's handle on the live cast.
type control struct {
	run  *cast
	sess *Session
}

func (c *control) SetVolume(ctx context.Context, level int) error {
	return c.sess.SetVolume(ctx, level)
}

// Pause stops the audio and drops what the receiver has buffered.
//
// The decoder upstream keeps running and its audio is discarded while paused,
// so resuming continues from where the music got to rather than from where it
// was interrupted. That is what a pushed stream can honestly offer: there is
// no queue on the receiver to hold a position in, and asking the source to
// stop is a different action belonging to the provider, not to this speaker.
func (c *control) Pause(ctx context.Context) error {
	c.run.setPaused(true)
	return c.sess.Flush(ctx)
}

func (c *control) Resume(ctx context.Context) error {
	c.run.setPaused(false)
	return nil
}

func (c *control) Playing() bool { return !c.run.isPaused() }
