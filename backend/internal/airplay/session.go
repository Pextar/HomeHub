package airplay

// One live connection to one receiver: the RTSP negotiation, the three UDP
// channels RAOP runs over, and the clock that keeps them honest.
//
//	audio    sender → receiver, one RTP packet per 352 frames
//	control  sender → receiver, a sync packet a second; receiver → sender,
//	         "I missed these packets, send them again"
//	timing   receiver → sender, "what time is it?", answered from the same
//	         clock every receiver in the cast is asking
//
// The timing channel is the whole reason this protocol beats pointing several
// speakers at one HTTP URL. There, each speaker fills a buffer and starts when
// it feels ready, and nothing afterwards corrects the offset between them.
// Here every receiver is told which sample belongs at which moment on a clock
// they can all measure against, so they start together and stay together.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// latencyFrames is how far ahead of playback the sender runs: two seconds,
// which is what AirPlay has always used and what receivers size their buffers
// for. It is the price of the sync — the buffer is what absorbs a Wi-Fi hiccup
// without a dropout — and it is why a tap on play takes a moment to be heard.
const latencyFrames = 2 * SampleRate

// syncInterval is how often the receiver is reminded where the clock is. Once
// a second is what senders have always done; more often wastes packets, less
// often lets a receiver's own crystal drift audibly before it is corrected.
const syncInterval = time.Second

// backlogPackets is how many sent packets are kept for retransmission — about
// eight seconds, comfortably longer than the two-second buffer a receiver
// could ask to have refilled.
const backlogPackets = 1024

// RTP payload types, as they appear on the wire with the marker/version bits
// already set.
const (
	ptAudio       = 0x60 // 96, the negotiated audio type
	ptAudioFirst  = 0xE0 // the same with the marker bit, on the first packet
	ptSync        = 0xD4 // 84 | marker
	ptTimingReply = 0xD3 // 83
	ptRetransmit  = 0xD6 // 86 | marker, wraps a resent audio packet
)

// Options tunes one session.
type Options struct {
	// Volume is the initial level, 0-100. Sent before the first packet so a
	// receiver does not start at whatever the last sender left it on.
	Volume int
	Logf   func(format string, args ...any)
}

// Session is a negotiated RAOP session with one receiver.
//
// Not safe for concurrent Send; everything else is. Send is called from a
// single pump goroutine per cast, which is also what keeps packet order
// meaningful.
type Session struct {
	dev   Device
	opts  Options
	rtsp  *conn
	codec Codec
	key   *cipherKey

	audio   *net.UDPConn
	control *net.UDPConn
	timing  *net.UDPConn
	// dstAudio/dstControl are where the receiver listens. The timing
	// channel is the other direction: the receiver asks, we answer to
	// whatever address the question came from.
	dstAudio   *net.UDPAddr
	dstControl *net.UDPAddr

	ssrc     uint32
	rtpStart uint32
	// seq and sent are written by the pump goroutine and read by the sync
	// loop and by anything that sends a control message mid-song, so they
	// are atomic rather than plain: the alternative is a mutex in the path
	// that runs 125 times a second per receiver.
	//
	// They are two counters rather than one because seq wraps at 16 bits
	// several times an hour and the timestamp must not.
	seq  atomic.Uint32
	sent atomic.Uint64
	// firstSent marks the marker bit as spent. Touched only by the pump
	// goroutine, which is the only caller of Send.
	firstSent bool

	mu      sync.Mutex
	backlog map[uint16][]byte
	order   []uint16

	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup
}

func (s *Session) logf(format string, args ...any) {
	if s.opts.Logf != nil {
		s.opts.Logf(format, args...)
	}
}

// Open negotiates a session with a receiver and leaves it ready for audio.
//
// Everything that can be refused is refused here rather than at the first
// packet: a receiver that is busy, wants a password, or will not take the
// codec says so during ANNOUNCE or SETUP, and the caller gets an error it can
// show instead of a stream that plays into nothing.
func Open(ctx context.Context, dev Device, opts Options) (*Session, error) {
	if ok, why := dev.Supported(); !ok {
		return nil, &UnsupportedError{Reason: fmt.Sprintf("%s: %s", dev.Name, why)}
	}
	rtsp, err := dial(ctx, dev.Addr())
	if err != nil {
		return nil, err
	}
	s := &Session{
		dev:      dev,
		opts:     opts,
		rtsp:     rtsp,
		ssrc:     randomUint32(),
		rtpStart: randomUint32(),
		backlog:  map[uint16][]byte{},
		done:     make(chan struct{}),
	}
	s.seq.Store(uint32(uint16(rand.Intn(1 << 16)))) //nolint:gosec // a sequence start, not a secret

	if err := s.negotiate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	if opts.Volume > 0 {
		// Best-effort: a receiver that refuses a volume change still plays.
		if err := s.SetVolume(ctx, opts.Volume); err != nil {
			s.logf("airplay: %s took the stream but refused a volume: %v", dev.Name, err)
		}
	}
	return s, nil
}

// negotiate runs OPTIONS → ANNOUNCE → SETUP → RECORD and opens the UDP
// channels in between, because SETUP has to name ports that already exist.
func (s *Session) negotiate(ctx context.Context) error {
	if _, err := s.rtsp.call(ctx, request{Method: "OPTIONS", URI: "*"}); err != nil {
		return err
	}

	uri := fmt.Sprintf("rtsp://%s/%d", s.rtsp.local, s.ssrc)
	if err := s.announce(ctx, uri); err != nil {
		return err
	}

	if err := s.openChannels(); err != nil {
		return err
	}
	setup, err := s.rtsp.call(ctx, request{
		Method: "SETUP",
		URI:    uri,
		Extra: map[string]string{
			"Transport": fmt.Sprintf(
				"RTP/AVP/UDP;unicast;interleaved=0;mode=record;control_port=%d;timing_port=%d",
				localPort(s.control), localPort(s.timing)),
		},
	})
	if err != nil {
		return err
	}
	audioPort, controlPort, _ := transportPorts(setup.Header("Transport"))
	if audioPort == 0 {
		return &UnsupportedError{
			Reason: fmt.Sprintf("%s accepted the session but named no audio port", s.dev.Name),
		}
	}
	s.dstAudio = &net.UDPAddr{IP: net.ParseIP(s.dev.IP), Port: audioPort}
	if controlPort > 0 {
		s.dstControl = &net.UDPAddr{IP: net.ParseIP(s.dev.IP), Port: controlPort}
	}

	if _, err := s.rtsp.call(ctx, request{
		Method: "RECORD",
		URI:    uri,
		Extra: map[string]string{
			"Range":    "npt=0-",
			"RTP-Info": fmt.Sprintf("seq=%d;rtptime=%d", s.sequence(), s.rtpStart),
		},
	}); err != nil {
		return err
	}

	s.wg.Add(2)
	go s.serveTiming()
	go s.serveControl()
	encrypted := ""
	if s.key != nil {
		encrypted = ", encrypted"
	}
	s.logf("airplay: %s ready (%s%s)", s.dev.Name, s.codec, encrypted)
	return nil
}

// announce offers the receiver a session, trying each shape the device might
// take until one is accepted.
//
// More than one attempt, because the mDNS advertisement is advice rather than
// an answer. An AirPlay 2 receiver publishes what an iPhone should use and may
// say nothing at all about the classic codecs — and a receiver that lists PCM
// but only implements ALAC exists too. Rather than refuse on a reading of the
// advertisement, each candidate is offered and the receiver's own refusal is
// what closes the door.
//
// The first refusal is the error that gets reported, not the last: it is the
// answer to the session the device asked for, so it is the one that explains
// what happened. Later attempts are HomeHub guessing.
func (s *Session) announce(ctx context.Context, uri string) error {
	attempts := s.dev.attempts()
	var first error

	for i, a := range attempts {
		s.codec = a.codec
		s.key = nil
		if a.cipher == EncryptionRSA {
			key, err := newCipherKey()
			if err != nil {
				return err
			}
			s.key = key
		}

		_, err := s.rtsp.call(ctx, request{
			Method:      "ANNOUNCE",
			URI:         uri,
			ContentType: "application/sdp",
			Body:        []byte(s.sdp()),
		})
		if err == nil {
			if i > 0 {
				s.logf("airplay: %s took %s on attempt %d", s.dev.Name, a.codec, i+1)
			}
			return nil
		}
		if first == nil {
			first = err
		}
		// A receiver that is busy, or wants a password, will say the same
		// thing to every offer. Retrying it three more times only delays the
		// error the user needs to read.
		var se *StatusError
		if errors.As(err, &se) && (se.Status == 401 || se.Status == 453) {
			return err
		}
		// Anything that is not the receiver answering — a dropped connection,
		// a timeout — means there is nothing left to negotiate with.
		if se == nil {
			return err
		}
	}
	return first
}

// sdp builds the session description. The fmtp line is ALAC's parameter set —
// frame length, bit depth, the compression tuning constants, sample rate — and
// is sent for both codecs because a receiver reads the frame length and depth
// from it either way.
func (s *Session) sdp() string {
	rtpmap := "AppleLossless"
	if s.codec == CodecPCM {
		rtpmap = fmt.Sprintf("L%d/%d/%d", BitsPerSample, SampleRate, Channels)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "v=0\r\n")
	fmt.Fprintf(&b, "o=HomeHub %d 0 IN IP4 %s\r\n", s.ssrc, s.rtsp.local)
	fmt.Fprintf(&b, "s=HomeHub\r\n")
	fmt.Fprintf(&b, "c=IN IP4 %s\r\n", s.dev.IP)
	fmt.Fprintf(&b, "t=0 0\r\n")
	fmt.Fprintf(&b, "m=audio 0 RTP/AVP 96\r\n")
	fmt.Fprintf(&b, "a=rtpmap:96 %s\r\n", rtpmap)
	fmt.Fprintf(&b, "a=fmtp:96 %d 0 %d 40 10 14 %d 255 0 0 %d\r\n",
		FramesPerPacket, BitsPerSample, Channels, SampleRate)
	if s.key != nil {
		fmt.Fprintf(&b, "a=rsaaeskey:%s\r\n", s.key.WrappedKey)
		fmt.Fprintf(&b, "a=aesiv:%s\r\n", s.key.IVBase64)
	}
	return b.String()
}

// openChannels binds the three local UDP sockets. They are bound before SETUP
// because SETUP has to tell the receiver which ports to answer on, and a port
// the kernel has not yet assigned cannot be named.
func (s *Session) openChannels() error {
	var err error
	if s.audio, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero}); err != nil {
		return fmt.Errorf("airplay: audio socket: %w", err)
	}
	if s.control, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero}); err != nil {
		return fmt.Errorf("airplay: control socket: %w", err)
	}
	if s.timing, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero}); err != nil {
		return fmt.Errorf("airplay: timing socket: %w", err)
	}
	return nil
}

func localPort(c *net.UDPConn) int {
	if c == nil {
		return 0
	}
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.Port
	}
	return 0
}

// Send transmits one packet of PCM. Anything shorter than a full packet is
// sent as-is — the tail of a stream — and anything longer is a caller bug the
// pump does not commit.
func (s *Session) Send(pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	payload := pcmPayload(pcm)
	if s.codec == CodecALAC {
		payload = alacPayload(pcm)
	}
	payload = s.key.encrypt(payload)

	pkt := make([]byte, 12+len(payload))
	pkt[0] = 0x80
	pkt[1] = ptAudio
	if !s.firstSent {
		pkt[1] = ptAudioFirst
		s.firstSent = true
	}
	seq := s.sequence()
	binary.BigEndian.PutUint16(pkt[2:4], seq)
	binary.BigEndian.PutUint32(pkt[4:8], s.rtpNow())
	binary.BigEndian.PutUint32(pkt[8:12], s.ssrc)
	copy(pkt[12:], payload)

	s.remember(seq, pkt)
	s.seq.Add(1)
	s.sent.Add(1)

	_, err := s.audio.WriteToUDP(pkt, s.dstAudio)
	if err != nil {
		return fmt.Errorf("airplay: sending to %s: %w", s.dev.Name, err)
	}
	return nil
}

// rtpNow is the timestamp of the packet about to be sent: the session's start
// plus one frame per frame already sent.
func (s *Session) rtpNow() uint32 {
	return s.rtpStart + uint32(s.sent.Load()*FramesPerPacket)
}

// sequence is the next packet's sequence number, truncated to the 16 bits the
// wire carries.
func (s *Session) sequence() uint16 { return uint16(s.seq.Load()) }

// remember keeps a packet for possible retransmission, dropping the oldest
// once the backlog is full.
func (s *Session) remember(seq uint16, pkt []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlog[seq] = pkt
	s.order = append(s.order, seq)
	if len(s.order) > backlogPackets {
		delete(s.backlog, s.order[0])
		s.order = s.order[1:]
	}
}

// Sync tells the receiver where the clock is. The first one after RECORD
// carries the marker bit, which is how a receiver knows a session has just
// (re)started rather than continued.
func (s *Session) Sync(first bool) error {
	if s.dstControl == nil {
		return nil // the receiver named no control port; nothing to sync to
	}
	now := s.rtpNow()
	pkt := make([]byte, 20)
	pkt[0] = 0x80
	if first {
		pkt[0] = 0x90
	}
	pkt[1] = ptSync
	binary.BigEndian.PutUint16(pkt[2:4], 7)
	// The sample the receiver should be playing right now, which is the one
	// sent a full latency ago. The gap between the two timestamps is how the
	// receiver learns how far ahead the sender is running.
	binary.BigEndian.PutUint32(pkt[4:8], now-latencyFrames)
	putNTP(pkt[8:16], ntpTime(time.Now().UnixNano()))
	binary.BigEndian.PutUint32(pkt[16:20], now)

	_, err := s.control.WriteToUDP(pkt, s.dstControl)
	return err
}

// serveTiming answers the receiver's clock questions. This is the loop that
// makes several receivers agree: each one measures its offset from the
// sender's clock, and the sender's clock is one clock.
func (s *Session) serveTiming() {
	defer s.wg.Done()
	buf := make([]byte, 64)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.timing.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := s.timing.ReadFromUDP(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return
		}
		if n < 32 {
			continue
		}
		received := ntpTime(time.Now().UnixNano())

		reply := make([]byte, 32)
		reply[0] = 0x80
		reply[1] = ptTimingReply
		binary.BigEndian.PutUint16(reply[2:4], 7)
		// Bytes 4-7 are zero. The three timestamps are the receiver's
		// question echoed back, when we heard it, and when we answered —
		// the same three an NTP exchange carries, and enough for the
		// receiver to solve for the offset and the round trip.
		copy(reply[8:16], buf[24:32])
		putNTP(reply[16:24], received)
		putNTP(reply[24:32], ntpTime(time.Now().UnixNano()))
		_, _ = s.timing.WriteToUDP(reply, from)
	}
}

// serveControl handles retransmission requests: the receiver naming packets it
// never got. Answering them is what keeps a Wi-Fi hiccup from becoming an
// audible gap, and it is cheap — the packets are already in memory.
func (s *Session) serveControl() {
	defer s.wg.Done()
	buf := make([]byte, 64)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.control.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := s.control.ReadFromUDP(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return
		}
		if n < 8 || buf[1]&0x7F != 0x55 { // 85: resend request
			continue
		}
		missed := binary.BigEndian.Uint16(buf[4:6])
		count := int(binary.BigEndian.Uint16(buf[6:8]))
		if count > backlogPackets {
			count = backlogPackets
		}
		for i := 0; i < count; i++ {
			seq := missed + uint16(i)
			s.mu.Lock()
			orig, ok := s.backlog[seq]
			s.mu.Unlock()
			if !ok {
				continue // older than the backlog; the receiver will cope
			}
			// A resend is the original packet wrapped in a four-byte
			// header, so the receiver can tell it apart from live audio.
			wrapped := make([]byte, 4+len(orig))
			wrapped[0] = 0x80
			wrapped[1] = ptRetransmit
			binary.BigEndian.PutUint16(wrapped[2:4], 1)
			copy(wrapped[4:], orig)
			_, _ = s.control.WriteToUDP(wrapped, from)
		}
	}
}

// SetVolume sets the receiver's own volume.
//
// AirPlay's scale is decibels of attenuation from -30 to 0, with -144 meaning
// silence — not a percentage, and not linear. Mapping 0-100 straight onto that
// range is what every sender does and what a user's ear expects from a slider
// that came from somewhere else in HomeHub.
func (s *Session) SetVolume(ctx context.Context, level int) error {
	db := -144.0
	if level > 0 {
		if level > 100 {
			level = 100
		}
		db = -30 + float64(level)*0.3
	}
	_, err := s.rtsp.call(ctx, request{
		Method:      "SET_PARAMETER",
		URI:         s.sessionURI(),
		ContentType: "text/parameters",
		Body:        []byte(fmt.Sprintf("volume: %f\r\n", db)),
	})
	return err
}

// SetMetadata tells the receiver what is playing, for its display.
func (s *Session) SetMetadata(ctx context.Context, title, artist, album string) error {
	if !s.dev.Metadata {
		return nil
	}
	body := daapMetadata(title, artist, album)
	if len(body) == 0 {
		return nil
	}
	_, err := s.rtsp.call(ctx, request{
		Method:      "SET_PARAMETER",
		URI:         s.sessionURI(),
		ContentType: daapContentType,
		Body:        body,
		Extra:       map[string]string{"RTP-Info": fmt.Sprintf("rtptime=%d", s.rtpNow())},
	})
	return err
}

// Flush stops playback and drops whatever the receiver still has buffered.
// This is AirPlay's pause: there is nothing on the receiver to pause, so the
// two seconds already sent have to be thrown away or they would play on after
// the button.
func (s *Session) Flush(ctx context.Context) error {
	_, err := s.rtsp.call(ctx, request{
		Method: "FLUSH",
		URI:    s.sessionURI(),
		Extra:  map[string]string{"RTP-Info": fmt.Sprintf("seq=%d;rtptime=%d", s.sequence(), s.rtpNow())},
	})
	return err
}

func (s *Session) sessionURI() string {
	return fmt.Sprintf("rtsp://%s/%d", s.rtsp.local, s.ssrc)
}

// Device is the receiver this session drives.
func (s *Session) Device() Device { return s.dev }

// Close tears the session down: TEARDOWN so the receiver frees itself for the
// next sender, then the sockets. Safe to call more than once.
func (s *Session) Close() {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.rtsp != nil {
			// Best-effort, and briefly: a receiver that has gone away must
			// not hold up shutting down the rest of the cast.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if s.dstAudio != nil {
				_, _ = s.rtsp.do(ctx, request{Method: "TEARDOWN", URI: s.sessionURI()})
			}
			cancel()
			_ = s.rtsp.Close()
		}
		for _, c := range []*net.UDPConn{s.audio, s.control, s.timing} {
			if c != nil {
				_ = c.Close()
			}
		}
		s.wg.Wait()

		s.mu.Lock()
		s.backlog = nil
		s.order = nil
		s.mu.Unlock()
	})
}

// isTimeout distinguishes the read deadline expiring — the normal way both
// listener loops check whether they have been told to stop — from the socket
// actually failing.
func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
