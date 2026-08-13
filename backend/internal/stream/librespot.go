package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"homehub/internal/media"
)

// Librespot decodes Spotify by running librespot as a Spotify Connect
// receiver and reading its PCM output.
//
// The shape of this is dictated by how Spotify works rather than by
// preference. There is no API that returns audio, so the only licensed
// decoder available is a Connect client — HomeHub registers as a device, the
// Web API points playback at that device, and the audio arrives as if HomeHub
// were a speaker. It then goes back out to the real speakers over HTTP.
//
// The consequence, stated here because it surprises people: while a mixed
// zone is playing, HomeHub *is* the account's active Spotify device. Starting
// Spotify on a phone takes the session away and the zone stops. That is the
// same single-session rule that makes the whole stream route necessary, seen
// from the other side.
type Librespot struct {
	cfg LibrespotConfig

	mu      sync.Mutex
	running *session
}

// LibrespotConfig configures the decoder.
type LibrespotConfig struct {
	// Binary is the librespot executable. Defaults to "librespot" resolved
	// on PATH.
	Binary string
	// DeviceName is what the receiver calls itself in Spotify's device list.
	// It is what the user will see and what they must not confuse with a
	// real speaker, so it says HomeHub.
	DeviceName string
	// CacheDir, when set, lets librespot keep its credentials and audio
	// cache, which makes the second start much faster than the first.
	CacheDir string
	// Bitrate is 96, 160 or 320. Defaults to 320: the audio is re-served
	// losslessly, so the decoder's bitrate is the only place quality is
	// lost, and there is no bandwidth reason to economise on a LAN.
	Bitrate int
	// StartTimeout caps how long to wait for the decoder to produce its
	// first audio before giving up.
	StartTimeout time.Duration
	Logf         func(format string, args ...any)
}

// DefaultDeviceName is what the receiver registers as when nothing else is
// configured.
const DefaultDeviceName = "HomeHub Multiroom"

// NewLibrespot creates a decoder. It does not start anything or check that
// librespot exists — call Available for that.
func NewLibrespot(cfg LibrespotConfig) *Librespot {
	if cfg.Binary == "" {
		cfg.Binary = "librespot"
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = DefaultDeviceName
	}
	if cfg.Bitrate == 0 {
		cfg.Bitrate = 320
	}
	if cfg.StartTimeout == 0 {
		cfg.StartTimeout = 20 * time.Second
	}
	return &Librespot{cfg: cfg}
}

func (l *Librespot) logf(format string, args ...any) {
	if l.cfg.Logf != nil {
		l.cfg.Logf(format, args...)
	}
}

// Available reports whether decoding could work, which for this decoder means
// one thing: is the binary there. Everything else — Premium, scopes — is the
// Spotify client's business and is reported separately, so a user missing one
// is not told about the other.
func (l *Librespot) Available() media.Availability {
	path, err := exec.LookPath(l.cfg.Binary)
	if err != nil {
		return media.Availability{
			Reason: fmt.Sprintf(
				"playing to speakers of different makes at once needs librespot on the HomeHub host, and %q isn't installed — see docs/MEDIA-PROTOCOL.md",
				l.cfg.Binary),
		}
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode()&0o111 == 0 {
		return media.Availability{
			Reason: fmt.Sprintf("librespot at %s isn't executable", path),
		}
	}
	return media.Availability{OK: true, Configured: true}
}

// session is one running librespot process.
type session struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	cancel context.CancelFunc

	closeOnce sync.Once
}

// Close stops the process and releases its pipe. Safe to call repeatedly.
func (s *session) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.stdout.Close()
		// Reap the process so it doesn't linger as a zombie. The context
		// cancel above has already signalled it; this only waits.
		_ = s.cmd.Wait()
	})
	return nil
}

// Read implements io.Reader by reading the decoder's PCM output.
func (s *session) Read(p []byte) (int, error) { return s.stdout.Read(p) }

// Open starts decoding. The returned stream is raw PCM, framed as WAV by the
// Host before it reaches a speaker.
//
// Only one decode runs at a time: librespot registers a Connect device, and
// two of them would show the user two identically named devices and fight
// over the account's single session. Opening a second stream therefore
// replaces the first, which matches what the user did — they started
// something new.
func (l *Librespot) Open(ctx context.Context, uri string) (*media.Stream, error) {
	if av := l.Available(); !av.OK {
		return nil, errors.New(av.Reason)
	}

	l.mu.Lock()
	if l.running != nil {
		l.logf("stream: replacing the running decoder")
		_ = l.running.Close()
		l.running = nil
	}
	l.mu.Unlock()

	// The process outlives the request that started it, so it gets its own
	// context rather than the caller's.
	procCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	cmd := exec.CommandContext(procCtx, l.cfg.Binary, l.args()...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stream: %w", err)
	}
	// librespot logs to stderr; forwarding it is how a user finds out that
	// their account isn't Premium, which is otherwise a silent failure.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stream: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("stream: starting librespot: %w", err)
	}
	go l.drainLogs(stderr)

	s := &session{cmd: cmd, stdout: stdout, cancel: cancel}
	l.mu.Lock()
	l.running = s
	l.mu.Unlock()

	l.logf("stream: librespot running as %q", l.cfg.DeviceName)
	// The body is raw S16LE at CD rate — the WAV header is prepended per
	// listener by the Host, not carried in the stream. Saying so lets the
	// AirPlay route pack these samples into RTP packets without parsing a
	// container that isn't there.
	pcm := media.CDQuality
	return &media.Stream{
		Body:        s,
		ContentType: ContentTypeWAV,
		PCM:         &pcm,
		Meta: media.Metadata{
			Title:       "Spotify",
			ContentType: ContentTypeWAV,
			Live:        true,
		},
	}, nil
}

// args builds the librespot command line.
//
// --backend pipe is the whole point: it makes librespot write raw PCM to
// stdout instead of to a sound card, which is what lets HomeHub re-serve it.
// A host with no audio hardware at all — a headless Pi — works fine this way.
func (l *Librespot) args() []string {
	args := []string{
		"--name", l.cfg.DeviceName,
		"--backend", "pipe",
		"--bitrate", fmt.Sprint(l.cfg.Bitrate),
		// Announce as a speaker so it sorts sensibly in Spotify's device
		// list next to the real ones.
		"--device-type", "speaker",
		// Without this librespot lowers volume on its own; the speakers do
		// their own volume and a second attenuation would compound.
		"--initial-volume", "100",
	}
	if l.cfg.CacheDir != "" {
		args = append(args, "--cache", l.cfg.CacheDir)
	}
	return args
}

// drainLogs forwards librespot's stderr, one line at a time. It must be
// consumed whether or not anyone is logging: an unread pipe fills and blocks
// the process.
func (l *Librespot) drainLogs(r io.ReadCloser) {
	defer func() { _ = r.Close() }()
	buf := make([]byte, 4096)
	var partial strings.Builder
	for {
		n, err := r.Read(buf)
		if n > 0 {
			partial.Write(buf[:n])
			text := partial.String()
			for {
				i := strings.IndexByte(text, '\n')
				if i < 0 {
					break
				}
				if line := strings.TrimSpace(text[:i]); line != "" {
					l.logf("librespot: %s", line)
				}
				text = text[i+1:]
			}
			partial.Reset()
			partial.WriteString(text)
		}
		if err != nil {
			return
		}
	}
}

// DeviceName is what the decoder registers as, so the Connect route can tell
// HomeHub's own receiver apart from a real speaker.
func (l *Librespot) DeviceName() string { return l.cfg.DeviceName }

// Close stops any running decoder. Called at shutdown.
func (l *Librespot) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running == nil {
		return nil
	}
	err := l.running.Close()
	l.running = nil
	return err
}
