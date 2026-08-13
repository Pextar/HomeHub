package airplay

import (
	"encoding/binary"
	"strings"
	"testing"
)

// The parser is fed bytes it will meet on a real network, so the tests build
// those bytes rather than exercising the parser through a socket. Everything
// below assembles a DNS message the way a responder would, including the
// compression pointers that are the part most likely to be got wrong.

// msgBuilder assembles a DNS response.
type msgBuilder struct {
	buf     []byte
	answers int
	// offsets remembers where each name was first written, so later records
	// can point at it the way a real responder compresses.
	offsets map[string]int
}

func newMsg() *msgBuilder {
	return &msgBuilder{buf: make([]byte, 12), offsets: map[string]int{}}
}

// name writes a domain name, compressing against anything already written.
func (m *msgBuilder) name(n string) {
	n = strings.TrimSuffix(n, ".") + "."
	for {
		if off, ok := m.offsets[n]; ok {
			m.buf = binary.BigEndian.AppendUint16(m.buf, uint16(0xC000|off))
			return
		}
		m.offsets[n] = len(m.buf)
		label, rest, found := strings.Cut(n, ".")
		if !found || label == "" {
			m.buf = append(m.buf, 0)
			return
		}
		m.buf = append(m.buf, byte(len(label)))
		m.buf = append(m.buf, label...)
		n = rest
		if n == "" {
			m.buf = append(m.buf, 0)
			return
		}
	}
}

// record writes one resource record, calling body to write its rdata.
func (m *msgBuilder) record(name string, typ uint16, body func()) {
	m.answers++
	m.name(name)
	m.buf = binary.BigEndian.AppendUint16(m.buf, typ)
	m.buf = binary.BigEndian.AppendUint16(m.buf, classIN)
	m.buf = binary.BigEndian.AppendUint32(m.buf, 120) // ttl
	lenAt := len(m.buf)
	m.buf = append(m.buf, 0, 0)
	start := len(m.buf)
	body()
	binary.BigEndian.PutUint16(m.buf[lenAt:lenAt+2], uint16(len(m.buf)-start))
}

func (m *msgBuilder) ptr(service, instance string) {
	m.record(service, typePTR, func() { m.name(instance) })
}

func (m *msgBuilder) srv(instance, target string, port int) {
	m.record(instance, typeSRV, func() {
		m.buf = binary.BigEndian.AppendUint16(m.buf, 0) // priority
		m.buf = binary.BigEndian.AppendUint16(m.buf, 0) // weight
		m.buf = binary.BigEndian.AppendUint16(m.buf, uint16(port))
		m.name(target)
	})
}

func (m *msgBuilder) txt(instance string, kv ...string) {
	m.record(instance, typeTXT, func() {
		for _, entry := range kv {
			m.buf = append(m.buf, byte(len(entry)))
			m.buf = append(m.buf, entry...)
		}
	})
}

func (m *msgBuilder) a(host string, ip [4]byte) {
	m.record(host, typeA, func() { m.buf = append(m.buf, ip[:]...) })
}

// bytes finalises the header. Everything is put in the answer section, which
// is the harder case for the parser than "additional" — the counts have to
// agree or the walk stops early.
func (m *msgBuilder) bytes() []byte {
	binary.BigEndian.PutUint16(m.buf[2:4], 0x8400) // response, authoritative
	binary.BigEndian.PutUint16(m.buf[6:8], uint16(m.answers))
	return m.buf
}

func TestBrowseParsesAFullAdvertisement(t *testing.T) {
	const instance = "B827EB1234AB@ropieee._raop._tcp.local."
	m := newMsg()
	m.ptr(raopService, instance)
	m.srv(instance, "ropieee.local.", 5000)
	m.txt(instance, "txtvers=1", "cn=0,1", "et=0,1", "sr=44100", "ss=16",
		"ch=2", "am=ShairportSync", "md=0,1,2", "pw=false")
	m.a("ropieee.local.", [4]byte{192, 168, 1, 42})

	recs := newRecords()
	recs.absorb(m.bytes(), "192.168.1.42")

	if got := recs.instances[raopService]; len(got) != 1 || got[0] != instance {
		t.Fatalf("instances = %v", got)
	}
	if s := recs.srv[instance]; s.port != 5000 || s.target != "ropieee.local." {
		t.Errorf("srv = %+v", s)
	}
	if got := recs.txt[instance]["am"]; got != "ShairportSync" {
		t.Errorf("txt am = %q", got)
	}
	if got := recs.address(instance); got != "192.168.1.42" {
		t.Errorf("address = %q", got)
	}
}

// The A record is the one responders most often send only to the multicast
// group, which a unicast-only scan never sees. The sender's own address is
// the same machine, so it stands in — without this, a working receiver is
// invisible on any network where multicast is filtered in one direction.
func TestAddressFallsBackToTheSender(t *testing.T) {
	const instance = "Study._raop._tcp.local."
	m := newMsg()
	m.ptr(raopService, instance)
	m.srv(instance, "study.local.", 7000)
	recs := newRecords()
	recs.absorb(m.bytes(), "10.0.0.7")

	if got := recs.address(instance); got != "10.0.0.7" {
		t.Errorf("address = %q, want the responder's own address", got)
	}
}

func TestParseMessageSkipsTheQuestionSection(t *testing.T) {
	// A response that echoes the question, which plenty of responders do.
	m := newMsg()
	m.ptr(raopService, "Kitchen._raop._tcp.local.")
	pkt := m.bytes()

	// Splice a question in: rebuild with qdcount=1 and the question bytes
	// between the header and the answers.
	q, err := buildQuery([]string{raopService})
	if err != nil {
		t.Fatal(err)
	}
	withQ := append([]byte{}, pkt[:12]...)
	withQ = append(withQ, q[12:]...)
	withQ = append(withQ, pkt[12:]...)
	binary.BigEndian.PutUint16(withQ[4:6], 1)

	// The answer's names were compressed against offsets from the original
	// packet, so this spliced message is only safe to parse for its counts;
	// what is asserted is that the question is skipped without error.
	if _, err := parseMessage(withQ); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// A malformed message must be dropped, not spun on. mDNS is a shared bus and
// not everything on it is well-formed or friendly.
func TestParserRejectsMalformedMessages(t *testing.T) {
	if _, err := parseMessage([]byte{1, 2, 3}); err == nil {
		t.Error("a short message must not parse")
	}

	// A name pointing at itself: the classic compression loop.
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[6:8], 1)
	loop := len(pkt)
	pkt = binary.BigEndian.AppendUint16(pkt, uint16(0xC000|loop))
	if _, _, err := readName(pkt, loop); err == nil {
		t.Error("a self-referential name must be refused")
	}
}

func TestBuildQueryAsksForUnicastReplies(t *testing.T) {
	q, err := buildQuery([]string{raopService, airplayService})
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint16(q[4:6]); got != 2 {
		t.Errorf("qdcount = %d, want 2", got)
	}
	// The unicast bit is what makes a one-shot browse from an ephemeral
	// port work at all; losing it would make discovery silently depend on
	// the multicast listener alone.
	if !strings.Contains(string(q), "_raop") {
		t.Fatal("the query should name the service")
	}
	class := binary.BigEndian.Uint16(q[len(q)-2:])
	if class&unicastBit == 0 {
		t.Errorf("qclass = %#x, want the unicast-response bit set", class)
	}
}

func TestParseTXTHandlesValuelessKeys(t *testing.T) {
	data := []byte{}
	for _, e := range []string{"cn=0,1", "bare", "k="} {
		data = append(data, byte(len(e)))
		data = append(data, e...)
	}
	got := parseTXT(data)
	if got["cn"] != "0,1" {
		t.Errorf("cn = %q", got["cn"])
	}
	if _, ok := got["bare"]; !ok {
		t.Error("a key with no = should still be recorded")
	}
	if v, ok := got["k"]; !ok || v != "" {
		t.Errorf("k = %q, %v", v, ok)
	}
}
