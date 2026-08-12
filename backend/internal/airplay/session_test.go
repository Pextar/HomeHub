package airplay

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A fake receiver, because the real ones are on somebody's shelf.
//
// It speaks enough RAOP to negotiate a session and receive audio: the RTSP
// exchange on TCP, and the three UDP channels. That covers the parts a sender
// gets wrong — the SDP it announces, the ports it asks for, the shape of the
// packets it sends — and leaves only the parts that need a real decoder, which
// no test can check anyway.

type fakeReceiver struct {
	ln    net.Listener
	audio *net.UDPConn

	mu       sync.Mutex
	requests []fakeRequest
	packets  [][]byte

	// refuse, when set, is the status every request after OPTIONS gets.
	refuse int
	// omitAudioPort drops server_port from the SETUP reply, which is how a
	// receiver that accepted the session but cannot take audio behaves.
	omitAudioPort bool

	done chan struct{}
	wg   sync.WaitGroup
}

type fakeRequest struct {
	Method  string
	URI     string
	Headers map[string]string
	Body    string
}

func newFakeReceiver(t *testing.T) *fakeReceiver {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	audio, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeReceiver{ln: ln, audio: audio, done: make(chan struct{})}
	f.wg.Add(2)
	go f.serve()
	go f.readAudio()
	t.Cleanup(f.close)
	return f
}

func (f *fakeReceiver) close() {
	select {
	case <-f.done:
		return
	default:
	}
	close(f.done)
	_ = f.ln.Close()
	_ = f.audio.Close()
	f.wg.Wait()
}

func (f *fakeReceiver) device() Device {
	host, port, _ := net.SplitHostPort(f.ln.Addr().String())
	n, _ := strconv.Atoi(port)
	return Device{
		Name: "Fake", IP: host, Port: n, ID: "aabbccddeeff",
		Codecs:     []Codec{CodecPCM, CodecALAC},
		Encryption: []Encryption{EncryptionNone},
		Audio:      Audio{SampleRate: SampleRate, BitDepth: BitsPerSample, Channels: Channels},
		Metadata:   true,
	}
}

func (f *fakeReceiver) serve() {
	defer f.wg.Done()
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer func() { _ = c.Close() }()
			f.handle(c)
		}()
	}
}

func (f *fakeReceiver) handle(c net.Conn) {
	br := bufio.NewReader(c)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		parts := strings.Fields(strings.TrimSpace(line))
		if len(parts) < 2 {
			return
		}
		req := fakeRequest{Method: parts[0], URI: parts[1], Headers: map[string]string{}}
		for {
			h, err := br.ReadString('\n')
			if err != nil {
				return
			}
			h = strings.TrimRight(h, "\r\n")
			if h == "" {
				break
			}
			if k, v, ok := strings.Cut(h, ":"); ok {
				req.Headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}
		if n, err := strconv.Atoi(req.Headers["content-length"]); err == nil && n > 0 {
			body := make([]byte, n)
			if _, err := io.ReadFull(br, body); err != nil {
				return
			}
			req.Body = string(body)
		}

		f.mu.Lock()
		f.requests = append(f.requests, req)
		refuse, omit := f.refuse, f.omitAudioPort
		f.mu.Unlock()

		if refuse != 0 && req.Method != "OPTIONS" {
			fmt.Fprintf(c, "RTSP/1.0 %d Refused\r\nCSeq: %s\r\n\r\n",
				refuse, req.Headers["cseq"])
			continue
		}

		var extra string
		switch req.Method {
		case "OPTIONS":
			extra = "Public: ANNOUNCE, SETUP, RECORD, FLUSH, TEARDOWN, SET_PARAMETER\r\n"
		case "SETUP":
			port := f.audio.LocalAddr().(*net.UDPAddr).Port
			if omit {
				extra = "Transport: RTP/AVP/UDP;unicast;mode=record\r\nSession: 1\r\n"
			} else {
				extra = fmt.Sprintf(
					"Transport: RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d\r\nSession: 1\r\n",
					port, port, port)
			}
		case "RECORD":
			extra = "Audio-Latency: 88200\r\n"
		}
		fmt.Fprintf(c, "RTSP/1.0 200 OK\r\nCSeq: %s\r\n%s\r\n", req.Headers["cseq"], extra)
	}
}

func (f *fakeReceiver) readAudio() {
	defer f.wg.Done()
	buf := make([]byte, 4096)
	for {
		select {
		case <-f.done:
			return
		default:
		}
		_ = f.audio.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := f.audio.ReadFromUDP(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		f.mu.Lock()
		f.packets = append(f.packets, pkt)
		f.mu.Unlock()
	}
}

func (f *fakeReceiver) request(method string) (fakeRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.Method == method {
			return r, true
		}
	}
	return fakeRequest{}, false
}

func (f *fakeReceiver) received() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.packets...)
}

func TestOpenNegotiatesAndAnnouncesTheRightSession(t *testing.T) {
	f := newFakeReceiver(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, err := Open(ctx, f.device(), Options{Volume: 40})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	ann, ok := f.request("ANNOUNCE")
	if !ok {
		t.Fatal("no ANNOUNCE was sent")
	}
	// The receiver takes PCM, so that is what is announced — and the fmtp
	// line has to be there either way, because receivers read the frame
	// length and bit depth from it.
	if !strings.Contains(ann.Body, "a=rtpmap:96 L16/44100/2") {
		t.Errorf("SDP should announce PCM:\n%s", ann.Body)
	}
	if !strings.Contains(ann.Body, "a=fmtp:96 352 0 16 40 10 14 2 255 0 0 44100") {
		t.Errorf("SDP is missing the ALAC parameter line:\n%s", ann.Body)
	}
	// Cleartext receiver: no key material should be offered at all.
	if strings.Contains(ann.Body, "rsaaeskey") {
		t.Errorf("a cleartext receiver should not be sent a key:\n%s", ann.Body)
	}

	setup, ok := f.request("SETUP")
	if !ok {
		t.Fatal("no SETUP was sent")
	}
	if !strings.Contains(setup.Headers["transport"], "mode=record") {
		t.Errorf("transport = %q", setup.Headers["transport"])
	}
	for _, want := range []string{"control_port=", "timing_port="} {
		if !strings.Contains(setup.Headers["transport"], want) {
			t.Errorf("transport should name its %s: %q", want, setup.Headers["transport"])
		}
	}
	if _, ok := f.request("RECORD"); !ok {
		t.Error("no RECORD was sent")
	}

	// The initial volume is sent before any audio, so a receiver left loud
	// by the last sender doesn't announce itself at full level.
	vol, ok := f.request("SET_PARAMETER")
	if !ok {
		t.Fatal("no initial volume was sent")
	}
	if !strings.HasPrefix(vol.Body, "volume: -18") { // -30 + 40×0.3
		t.Errorf("volume body = %q, want the dB scale", vol.Body)
	}
}

func TestOpenRefusesAReceiverThatNamesNoAudioPort(t *testing.T) {
	f := newFakeReceiver(t)
	f.mu.Lock()
	f.omitAudioPort = true
	f.mu.Unlock()

	_, err := Open(context.Background(), f.device(), Options{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "no audio port") {
		t.Errorf("error should say what was missing: %v", err)
	}
}

// 453 is the status a receiver returns when another sender already has it.
// It reaches the user, so it has to be a sentence rather than a number.
func TestBusyReceiverExplainsItself(t *testing.T) {
	f := newFakeReceiver(t)
	f.mu.Lock()
	f.refuse = 453
	f.mu.Unlock()

	_, err := Open(context.Background(), f.device(), Options{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "already playing something") {
		t.Errorf("error = %v", err)
	}
}

func TestUnsupportedReceiverIsRefusedBeforeConnecting(t *testing.T) {
	dev := Device{Name: "Apple TV", IP: "127.0.0.1", Port: 1,
		Codecs: []Codec{CodecAAC}, Encryption: []Encryption{EncryptionFairPlay}}
	_, err := Open(context.Background(), dev, Options{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "Apple TV") {
		t.Errorf("the error should name the receiver: %v", err)
	}
}

func TestSendProducesWellFormedRTP(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	first := sess.sequence()
	start := sess.rtpStart
	for i := 0; i < 3; i++ {
		if err := sess.Send(samples(FramesPerPacket)); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	var got [][]byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got = f.received(); len(got) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) < 3 {
		t.Fatalf("received %d packets, want 3", len(got))
	}

	// The marker bit on the first packet is how a receiver knows a session
	// has just started rather than continued.
	if got[0][1] != ptAudioFirst {
		t.Errorf("first packet type = %#x, want %#x", got[0][1], ptAudioFirst)
	}
	if got[1][1] != ptAudio {
		t.Errorf("later packet type = %#x, want %#x", got[1][1], ptAudio)
	}
	for i, pkt := range got[:3] {
		if pkt[0] != 0x80 {
			t.Errorf("packet %d: version byte = %#x", i, pkt[0])
		}
		if seq := binary.BigEndian.Uint16(pkt[2:4]); seq != first+uint16(i) {
			t.Errorf("packet %d: seq = %d, want %d", i, seq, first+uint16(i))
		}
		// The timestamp advances by exactly one packet of frames, which is
		// what the receiver uses to place the audio on the shared clock.
		if ts := binary.BigEndian.Uint32(pkt[4:8]); ts != start+uint32(i*FramesPerPacket) {
			t.Errorf("packet %d: rtptime = %d, want %d", i, ts, start+uint32(i*FramesPerPacket))
		}
		if want := 12 + PacketBytes; len(pkt) != want {
			t.Errorf("packet %d: %d bytes, want %d", i, len(pkt), want)
		}
	}
}

func TestSyncPacketCarriesBothTimestamps(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	// The fake reports one port for all three channels, so the sync lands
	// in the same capture as the audio.
	if err := sess.Sync(true); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var pkt []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && pkt == nil {
		for _, p := range f.received() {
			if len(p) == 20 && p[1] == ptSync {
				pkt = p
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pkt == nil {
		t.Fatal("no sync packet arrived")
	}
	if pkt[0] != 0x90 {
		t.Errorf("first sync should carry the marker bit, got %#x", pkt[0])
	}
	playing := binary.BigEndian.Uint32(pkt[4:8])
	now := binary.BigEndian.Uint32(pkt[16:20])
	if now-playing != latencyFrames {
		t.Errorf("latency between the timestamps = %d, want %d", now-playing, latencyFrames)
	}
	if binary.BigEndian.Uint64(pkt[8:16]) == 0 {
		t.Error("the sync packet must carry an NTP time")
	}
}

// A receiver asking what time it is must get an answer with its own question
// echoed back, or it cannot solve for the offset — and without the offset it
// never starts playing.
func TestTimingRequestsAreAnswered(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	client, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP: net.IPv4(127, 0, 0, 1), Port: localPort(sess.timing)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	req := make([]byte, 32)
	req[0] = 0x80
	req[1] = 0xD2 // timing request
	binary.BigEndian.PutUint16(req[2:4], 7)
	origin := ntpTime(time.Now().UnixNano())
	putNTP(req[24:32], origin)
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	reply := make([]byte, 64)
	n, err := client.Read(reply)
	if err != nil {
		t.Fatalf("no timing reply: %v", err)
	}
	if n != 32 {
		t.Fatalf("reply is %d bytes, want 32", n)
	}
	if reply[1] != ptTimingReply {
		t.Errorf("reply type = %#x, want %#x", reply[1], ptTimingReply)
	}
	if got := binary.BigEndian.Uint64(reply[8:16]); got != origin {
		t.Errorf("origin timestamp = %d, want the question's %d", got, origin)
	}
	if binary.BigEndian.Uint64(reply[16:24]) == 0 || binary.BigEndian.Uint64(reply[24:32]) == 0 {
		t.Error("receive and transmit timestamps must be filled in")
	}
}

// A packet lost on Wi-Fi is a gap in the music unless the sender can resend
// it. The backlog is what makes that possible.
func TestRetransmitRequestsAreServedFromTheBacklog(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	wanted := sess.sequence()
	if err := sess.Send(samples(FramesPerPacket)); err != nil {
		t.Fatalf("send: %v", err)
	}

	client, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP: net.IPv4(127, 0, 0, 1), Port: localPort(sess.control)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	req := make([]byte, 8)
	req[0] = 0x80
	req[1] = 0xD5 // resend request
	binary.BigEndian.PutUint16(req[2:4], 1)
	binary.BigEndian.PutUint16(req[4:6], wanted)
	binary.BigEndian.PutUint16(req[6:8], 1)
	if _, err := client.Write(req); err != nil {
		t.Fatal(err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	reply := make([]byte, 2048)
	n, err := client.Read(reply)
	if err != nil {
		t.Fatalf("no resend: %v", err)
	}
	if reply[1] != ptRetransmit {
		t.Errorf("resend type = %#x, want %#x", reply[1], ptRetransmit)
	}
	// The original packet follows a four-byte wrapper, sequence intact.
	if got := binary.BigEndian.Uint16(reply[6:8]); got != wanted {
		t.Errorf("resent seq = %d, want %d", got, wanted)
	}
	if want := 4 + 12 + PacketBytes; n != want {
		t.Errorf("resend is %d bytes, want %d", n, want)
	}
}

func TestCloseTearsTheSessionDown(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess.Close()
	sess.Close() // must be safe twice

	if _, ok := f.request("TEARDOWN"); !ok {
		t.Error("closing should release the receiver for the next sender")
	}
}

func TestTransportPortsParsesTheHeader(t *testing.T) {
	server, control, timing := transportPorts(
		"RTP/AVP/UDP;unicast;mode=record;server_port=6000;control_port=6001;timing_port=6002")
	if server != 6000 || control != 6001 || timing != 6002 {
		t.Errorf("got %d/%d/%d", server, control, timing)
	}
	// A header with nothing usable must not be read as port zero somewhere.
	if s, c, ti := transportPorts("RTP/AVP/UDP;unicast"); s|c|ti != 0 {
		t.Errorf("got %d/%d/%d, want zeroes", s, c, ti)
	}
}

func TestVolumeMapsOntoTheDecibelScale(t *testing.T) {
	f := newFakeReceiver(t)
	sess, err := Open(context.Background(), f.device(), Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	cases := []struct {
		level int
		want  string
	}{
		{0, "volume: -144"},  // silence is its own value, not -30
		{100, "volume: 0."},  // the top of the scale
		{50, "volume: -15."}, // halfway
	}
	for _, tc := range cases {
		if err := sess.SetVolume(context.Background(), tc.level); err != nil {
			t.Fatalf("set volume %d: %v", tc.level, err)
		}
		f.mu.Lock()
		last := f.requests[len(f.requests)-1]
		f.mu.Unlock()
		if !strings.HasPrefix(last.Body, tc.want) {
			t.Errorf("level %d sent %q, want %q…", tc.level, last.Body, tc.want)
		}
	}
}
