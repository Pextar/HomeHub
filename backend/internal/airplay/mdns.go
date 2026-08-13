package airplay

// A one-shot mDNS (DNS-SD) browser, hand-rolled for the same reason the SSDP
// probes in internal/sonos and internal/kef are: the query is three records
// long, the answers are a handful of record types, and a dependency that
// brings a responder, a cache and a goroutine pool with it would be more code
// in the tree than this file, not less.
//
// It is a browse, not a resolver. One query goes out, whatever arrives inside
// the wait window is parsed, and the socket closes. Nothing is cached between
// scans: a receiver that was unplugged since the last scan should disappear
// from the next one, and the user pressing "scan again" means exactly that.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// mdnsGroup is the IPv4 mDNS multicast address and port.
const mdnsGroup = "224.0.0.251:5353"

// Service names browsed. Both are asked in one query because a receiver may
// advertise either or both, and a second round trip to find that out would
// double the scan for no new information.
const (
	// raopService is the audio receiver — what actually takes a stream.
	raopService = "_raop._tcp.local."
	// airplayService carries the friendly name and the model. shairport-sync
	// registers its RAOP instance as "<MAC>@<name>", so this is mostly
	// corroboration, but an Apple TV's RAOP name is not always its display
	// name and this is where that comes from.
	airplayService = "_airplay._tcp.local."
)

// DNS record types used here.
const (
	typeA   = 1
	typePTR = 12
	typeTXT = 16
	typeSRV = 33
)

// classIN is the internet class. The top bit of the class field in a *question*
// is mDNS's "unicast reply wanted" flag, which is how a one-shot browser from
// an ephemeral port hears anything at all.
const (
	classIN    = 1
	unicastBit = 0x8000
)

// browse sends one query for every service named and collects the records that
// come back until the window closes.
//
// Two sockets, because the two ways a responder can answer are mutually
// exclusive per socket. The query carries the unicast-reply bit, so
// well-behaved responders answer straight back to the ephemeral port — that is
// the path that works when a firewall or a VLAN eats multicast in one
// direction. Responders that ignore the bit answer to the group instead, which
// the second socket is joined to. Both are best-effort: a host where binding
// 5353 is refused (avahi without address reuse, a locked-down container) still
// gets the unicast half, which is why the multicast listener's error is
// dropped rather than returned.
func browse(ctx context.Context, wait time.Duration, services ...string) (*records, error) {
	if wait <= 0 {
		wait = 2 * time.Second
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	query, err := buildQuery(services)
	if err != nil {
		return nil, err
	}

	dst, err := net.ResolveUDPAddr("udp4", mdnsGroup)
	if err != nil {
		return nil, err
	}
	uni, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("airplay: mdns socket: %w", err)
	}
	defer func() { _ = uni.Close() }()

	// Joined for the responders that answer to the group. Failure is normal
	// on a host that already runs a responder without address reuse.
	multi, _ := net.ListenMulticastUDP("udp4", nil, dst)
	if multi != nil {
		defer func() { _ = multi.Close() }()
	}

	// Three sends spaced out. mDNS is UDP on a home network; losing one
	// datagram is routine, and a receiver missed because of it looks to the
	// user exactly like a receiver that is switched off.
	go func() {
		for i := 0; i < 3; i++ {
			if _, err := uni.WriteToUDP(query, dst); err != nil {
				return
			}
			select {
			case <-time.After(250 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
	}()

	recs := newRecords()
	done := make(chan struct{})
	collect := func(c *net.UDPConn) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 9000) // an mDNS response can be a full jumbo of records
		for {
			_ = c.SetReadDeadline(deadline)
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return // deadline, or the socket closed under us
			}
			var src string
			if from != nil && from.IP != nil {
				src = from.IP.String()
			}
			recs.absorb(buf[:n], src)
		}
	}

	go collect(uni)
	waiting := 1
	if multi != nil {
		waiting++
		go collect(multi)
	}
	for i := 0; i < waiting; i++ {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	return recs, nil
}

// buildQuery assembles one DNS query carrying a PTR question per service.
func buildQuery(services []string) ([]byte, error) {
	if len(services) == 0 {
		return nil, errors.New("airplay: no services to browse")
	}
	msg := make([]byte, 12, 128)
	// ID 0 is conventional for mDNS: replies are matched by name, not id.
	binary.BigEndian.PutUint16(msg[4:6], uint16(len(services))) // QDCOUNT
	for _, svc := range services {
		name, err := encodeName(svc)
		if err != nil {
			return nil, err
		}
		msg = append(msg, name...)
		msg = binary.BigEndian.AppendUint16(msg, typePTR)
		msg = binary.BigEndian.AppendUint16(msg, classIN|unicastBit)
	}
	return msg, nil
}

// encodeName writes a domain name in DNS label form. No compression: a query
// this small has nothing to compress against.
func encodeName(name string) ([]byte, error) {
	out := make([]byte, 0, len(name)+2)
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 {
			continue
		}
		if len(label) > 63 {
			return nil, fmt.Errorf("airplay: label %q too long", label)
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0), nil
}

// srv is the useful half of an SRV record.
type srv struct {
	target string
	port   int
}

// records is everything a browse heard, indexed the way assembling a device
// needs it. Answers arrive spread across several packets from several hosts,
// with no guarantee that an instance's SRV, TXT and A come together — so they
// are collected flat and joined afterwards.
type records struct {
	// instances maps a service name to the instance names advertised under
	// it ("_raop._tcp.local." → "B827EB…@Kitchen._raop._tcp.local.").
	instances map[string][]string
	srv       map[string]srv               // instance FQDN → host + port
	txt       map[string]map[string]string // instance FQDN → TXT key/values
	addr      map[string]string            // hostname → IPv4
	// from remembers which host an instance's records arrived from, the
	// fallback address for a responder that answered without an A record.
	from map[string]string
}

func newRecords() *records {
	return &records{
		instances: map[string][]string{},
		srv:       map[string]srv{},
		txt:       map[string]map[string]string{},
		addr:      map[string]string{},
		from:      map[string]string{},
	}
}

// absorb parses one response packet into the index. A packet that doesn't
// parse is dropped whole: mDNS is a shared bus and not everything on it is
// meant for us.
func (r *records) absorb(pkt []byte, src string) {
	msg, err := parseMessage(pkt)
	if err != nil {
		return
	}
	for _, rr := range msg {
		switch rr.typ {
		case typePTR:
			svc, instance := strings.ToLower(rr.name), rr.ptr
			if instance == "" {
				continue
			}
			if !contains(r.instances[svc], instance) {
				r.instances[svc] = append(r.instances[svc], instance)
			}
			if src != "" {
				r.from[instance] = src
			}
		case typeSRV:
			if rr.srv.target != "" {
				r.srv[rr.name] = rr.srv
			}
			if src != "" {
				r.from[rr.name] = src
			}
		case typeTXT:
			if len(rr.txt) > 0 {
				r.txt[rr.name] = rr.txt
			}
			if src != "" {
				r.from[rr.name] = src
			}
		case typeA:
			if rr.a != "" {
				r.addr[strings.ToLower(rr.name)] = rr.a
			}
		}
	}
}

// address resolves an instance to an IPv4 address: its SRV target's A record
// when one was published, otherwise the address the records arrived from.
// The fallback matters — plenty of responders send the A record only to the
// multicast group, which a unicast-only scan never sees, and the packet's
// source address is the same machine by definition.
func (r *records) address(instance string) string {
	if s, ok := r.srv[instance]; ok {
		if ip, ok := r.addr[strings.ToLower(s.target)]; ok {
			return ip
		}
	}
	return r.from[instance]
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// resource is one parsed record, reduced to the fields this package reads.
type resource struct {
	name string
	typ  uint16
	ptr  string
	srv  srv
	txt  map[string]string
	a    string
}

// parseMessage reads every resource record in a DNS message, skipping the
// question section. Both answers and the additional section are returned —
// mDNS responders put the SRV, TXT and A records that complete a PTR answer in
// "additional", and ignoring that section would mean a second query per
// device.
func parseMessage(pkt []byte) ([]resource, error) {
	if len(pkt) < 12 {
		return nil, errors.New("airplay: short dns message")
	}
	qd := int(binary.BigEndian.Uint16(pkt[4:6]))
	counts := int(binary.BigEndian.Uint16(pkt[6:8])) + // answers
		int(binary.BigEndian.Uint16(pkt[8:10])) + // authority
		int(binary.BigEndian.Uint16(pkt[10:12])) // additional

	off := 12
	for i := 0; i < qd; i++ {
		var err error
		if _, off, err = readName(pkt, off); err != nil {
			return nil, err
		}
		if off+4 > len(pkt) {
			return nil, errors.New("airplay: truncated question")
		}
		off += 4 // qtype + qclass
	}

	out := make([]resource, 0, counts)
	for i := 0; i < counts; i++ {
		name, next, err := readName(pkt, off)
		if err != nil {
			break // a truncated tail still leaves the records already read
		}
		off = next
		if off+10 > len(pkt) {
			break
		}
		typ := binary.BigEndian.Uint16(pkt[off : off+2])
		dataLen := int(binary.BigEndian.Uint16(pkt[off+8 : off+10]))
		off += 10
		if off+dataLen > len(pkt) {
			break
		}
		data := pkt[off : off+dataLen]
		off += dataLen

		rr := resource{name: name, typ: typ}
		switch typ {
		case typePTR:
			if v, _, err := readName(pkt, off-dataLen); err == nil {
				rr.ptr = v
			}
		case typeSRV:
			if len(data) >= 6 {
				port := int(binary.BigEndian.Uint16(data[4:6]))
				if target, _, err := readName(pkt, off-dataLen+6); err == nil {
					rr.srv = srv{target: target, port: port}
				}
			}
		case typeTXT:
			rr.txt = parseTXT(data)
		case typeA:
			if len(data) == 4 {
				rr.a = net.IP(data).String()
			}
		default:
			continue // AAAA, NSEC and the rest: nothing here reads them
		}
		out = append(out, rr)
	}
	return out, nil
}

// maxNameJumps bounds compression-pointer following. A message that points in
// a loop is malformed or hostile; either way the parse stops rather than
// spinning.
const maxNameJumps = 16

// readName decodes a possibly-compressed domain name, returning the name and
// the offset just past it in the *original* stream (pointers do not advance
// the caller's position beyond the two-byte pointer itself).
func readName(pkt []byte, off int) (string, int, error) {
	var labels []string
	jumps := 0
	next := -1
	for {
		if off < 0 || off >= len(pkt) {
			return "", 0, errors.New("airplay: name out of range")
		}
		n := int(pkt[off])
		switch {
		case n == 0:
			off++
			if next < 0 {
				next = off
			}
			return strings.Join(labels, ".") + ".", next, nil
		case n&0xC0 == 0xC0:
			if off+1 >= len(pkt) {
				return "", 0, errors.New("airplay: truncated pointer")
			}
			ptr := int(binary.BigEndian.Uint16(pkt[off:off+2]) &^ 0xC000)
			if next < 0 {
				next = off + 2
			}
			jumps++
			if jumps > maxNameJumps {
				return "", 0, errors.New("airplay: name pointer loop")
			}
			off = ptr
		default:
			if off+1+n > len(pkt) {
				return "", 0, errors.New("airplay: truncated label")
			}
			labels = append(labels, string(pkt[off+1:off+1+n]))
			off += 1 + n
		}
	}
}

// parseTXT reads the length-prefixed "key=value" strings of a TXT record. A
// key with no "=" is recorded with an empty value, which is how boolean-ish
// keys are sometimes written.
func parseTXT(data []byte) map[string]string {
	out := make(map[string]string)
	for i := 0; i < len(data); {
		n := int(data[i])
		i++
		if n == 0 || i+n > len(data) {
			if n == 0 {
				continue
			}
			break
		}
		entry := string(data[i : i+n])
		i += n
		if k, v, ok := strings.Cut(entry, "="); ok {
			out[k] = v
		} else {
			out[entry] = ""
		}
	}
	return out
}
