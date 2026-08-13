package airplay

// The RTSP half of RAOP: the control channel a sender uses to negotiate a
// session before any audio moves. It is HTTP-shaped but not HTTP — the version
// token is RTSP/1.0, the methods are ANNOUNCE/SETUP/RECORD/FLUSH/TEARDOWN, and
// the response to a request is matched by its CSeq rather than by order — so
// net/http cannot carry it and this is a small purpose-built client instead.

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// rtspTimeout bounds one request/response exchange. Generous enough for a
// receiver that has to spin up an output device on ANNOUNCE, short enough that
// a box which stopped answering fails a play rather than hanging it.
const rtspTimeout = 10 * time.Second

// userAgent is what HomeHub calls itself on the control channel. Receivers log
// it, and shairport-sync shows it in its own status, so it says HomeHub rather
// than impersonating iTunes.
const userAgent = "HomeHub/1 (AirPlay)"

// StatusError is a receiver refusing a request. Kept as a type because the
// status is the actionable part: 453 means something else is already playing
// to this receiver, which is a sentence the user can act on, and 401 means it
// wants a password.
type StatusError struct {
	Method string
	Status int
	Reason string
}

func (e *StatusError) Error() string {
	switch e.Status {
	case 401:
		return "the receiver asked for a password"
	case 453:
		return "the receiver is already playing something from somewhere else"
	}
	return fmt.Sprintf("the receiver refused %s: %d %s", e.Method, e.Status, e.Reason)
}

// UnsupportedError is a receiver this package cannot drive, with the reason in
// words meant for a person.
type UnsupportedError struct{ Reason string }

func (e *UnsupportedError) Error() string { return e.Reason }

// request is one RTSP request.
type request struct {
	Method      string
	URI         string
	ContentType string
	Body        []byte
	// Extra are headers beyond the ones every request carries.
	Extra map[string]string
}

// response is one RTSP response.
type response struct {
	Status  int
	Reason  string
	Headers map[string]string
	Body    []byte
}

// Header reads a header case-insensitively, returning "" when absent.
func (r *response) Header(name string) string {
	if r == nil {
		return ""
	}
	return r.Headers[strings.ToLower(name)]
}

// conn is one RTSP control connection to a receiver.
type conn struct {
	net     net.Conn
	br      *bufio.Reader
	cseq    int
	session string

	// The identity headers a RAOP receiver expects on every request. They
	// are how it knows two requests came from the same sender, and how a
	// remote-control app would find its way back; HomeHub sends stable
	// random values per connection and offers no remote endpoint.
	instance     string
	dacpID       string
	activeRemote uint32

	// local is the address the receiver sees us on, which goes into the
	// session URI and the SDP. Read from the socket rather than guessed:
	// on a multi-homed host the right answer is the interface that reached
	// this receiver, which is exactly what the kernel picked.
	local string
}

// dial opens the control connection.
func dial(ctx context.Context, addr string) (*conn, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("airplay: reaching %s: %w", addr, err)
	}
	local := ""
	if host, _, err := net.SplitHostPort(c.LocalAddr().String()); err == nil {
		local = host
	}
	return &conn{
		net:          c,
		br:           bufio.NewReader(c),
		instance:     randomHex(8),
		dacpID:       strings.ToUpper(randomHex(8)),
		activeRemote: randomUint32(),
		local:        local,
	}, nil
}

func (c *conn) Close() error { return c.net.Close() }

// do sends one request and reads its response.
func (c *conn) do(ctx context.Context, req request) (*response, error) {
	deadline := time.Now().Add(rtspTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.net.SetDeadline(deadline); err != nil {
		return nil, err
	}

	c.cseq++
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s RTSP/1.0\r\n", req.Method, req.URI)
	fmt.Fprintf(&b, "CSeq: %d\r\n", c.cseq)
	fmt.Fprintf(&b, "User-Agent: %s\r\n", userAgent)
	fmt.Fprintf(&b, "Client-Instance: %s\r\n", c.instance)
	fmt.Fprintf(&b, "DACP-ID: %s\r\n", c.dacpID)
	fmt.Fprintf(&b, "Active-Remote: %d\r\n", c.activeRemote)
	if c.session != "" {
		fmt.Fprintf(&b, "Session: %s\r\n", c.session)
	}
	// Sorted so a request is byte-identical run to run, which is what makes
	// the wire format testable.
	keys := make([]string, 0, len(req.Extra))
	for k := range req.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\r\n", k, req.Extra[k])
	}
	if len(req.Body) > 0 {
		if req.ContentType != "" {
			fmt.Fprintf(&b, "Content-Type: %s\r\n", req.ContentType)
		}
		fmt.Fprintf(&b, "Content-Length: %d\r\n", len(req.Body))
	}
	b.WriteString("\r\n")

	if _, err := io.WriteString(c.net, b.String()); err != nil {
		return nil, fmt.Errorf("airplay: sending %s: %w", req.Method, err)
	}
	if len(req.Body) > 0 {
		if _, err := c.net.Write(req.Body); err != nil {
			return nil, fmt.Errorf("airplay: sending %s body: %w", req.Method, err)
		}
	}

	resp, err := readResponse(c.br)
	if err != nil {
		return nil, fmt.Errorf("airplay: reading %s response: %w", req.Method, err)
	}
	if s := resp.Header("Session"); s != "" && c.session == "" {
		// The session id may carry parameters ("Session: 1;timeout=60").
		c.session, _, _ = strings.Cut(s, ";")
		c.session = strings.TrimSpace(c.session)
	}
	return resp, nil
}

// call is do plus the status check every caller would otherwise write.
func (c *conn) call(ctx context.Context, req request) (*response, error) {
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return nil, &StatusError{Method: req.Method, Status: resp.Status, Reason: resp.Reason}
	}
	return resp, nil
}

// readResponse parses a status line, headers and a Content-Length body.
func readResponse(br *bufio.Reader) (*response, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(strings.TrimRight(line, "\r\n"), " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(strings.ToUpper(parts[0]), "RTSP/") {
		return nil, fmt.Errorf("not an RTSP response: %q", strings.TrimSpace(line))
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("bad status %q", parts[1])
	}
	resp := &response{Status: status, Headers: map[string]string{}}
	if len(parts) == 3 {
		resp.Reason = parts[2]
	}

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		resp.Headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	if n, err := strconv.Atoi(resp.Header("Content-Length")); err == nil && n > 0 {
		body := make([]byte, n)
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, err
		}
		resp.Body = body
	}
	return resp, nil
}

// transportPorts pulls the receiver's UDP ports out of a SETUP response's
// Transport header: "RTP/AVP/UDP;unicast;mode=record;server_port=6000;
// control_port=6001;timing_port=6002".
func transportPorts(header string) (server, control, timing int) {
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "server_port":
			server = n
		case "control_port":
			control = n
		case "timing_port":
			timing = n
		}
	}
	return server, control, timing
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is fatal elsewhere in the process; here the
		// value is only an identity token, so a fixed one is better than a
		// panic in the middle of pressing play.
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

func randomUint32() uint32 {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return 1
	}
	return binary.BigEndian.Uint32(b)
}
