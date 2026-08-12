package media

// AirPlay is the one route where HomeHub does not hand content to a speaker
// but pushes it at one. That inverts who owns the audio, so it needs a little
// more vocabulary than the other routes: a description of the receiver good
// enough to open a session with, a host that owns the sending, and a way to
// reach a receiver that is already being driven.
//
// None of the protocol lives here. This package knows that a receiver has an
// address and takes PCM or ALAC; internal/airplay knows what any of that means
// on the wire. That split is the same one StreamHost draws, and it is what
// keeps the route engine testable without a socket.

import "context"

// AirPlayDest is where and how to reach one receiver. Built by the endpoint
// adapter from what the receiver advertised, and passed through the host
// untouched.
type AirPlayDest struct {
	// ID is the endpoint id, which is also how a live cast is looked up
	// again when someone changes the volume mid-song.
	ID   string
	Name string
	Host string
	Port int
	// PCM and ALAC are the formats the receiver said it takes. Both are
	// bit-exact; which is used is the sender's choice.
	PCM  bool
	ALAC bool
	// NeedsEncryption is a receiver that will not accept cleartext audio.
	NeedsEncryption bool
	// Metadata is whether the receiver has a display to fill in.
	Metadata bool
	// Volume is the level to set before audio starts, 0-100, and zero means
	// "leave it where it is". Per receiver rather than per cast because an
	// AirPlay receiver keeps its own volume between senders: the level that
	// belongs to a kitchen box is not the one that belongs to a study box,
	// and a cast that levelled them together would undo whatever the
	// household had set.
	Volume int
}

// AirPlayTarget is CapAirPlay: the endpoint can be sent a pushed stream.
type AirPlayTarget interface {
	AirPlayDest() AirPlayDest
}

// AirPlayControl is a receiver inside a running cast.
//
// It exists because an AirPlay receiver has no state of its own to read or
// change: it is a sink, and everything about what it is playing lives in the
// sender. Volume is the exception — that is the receiver's own — and pause
// means "stop sending and drop what is buffered", which only the sender can
// do.
type AirPlayControl interface {
	SetVolume(ctx context.Context, level int) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Playing() bool
}

// AirPlayHost sends audio to receivers. Implemented by internal/airplay.
type AirPlayHost interface {
	// Cast pushes s to every destination and returns a stop function that
	// ends the cast and releases the receivers. All destinations or none:
	// a partial cast is silence in half a room with no error to explain it.
	Cast(ctx context.Context, s *Stream, dests []AirPlayDest) (stop func(), err error)
	// Live returns the control surface for an endpoint the running cast is
	// driving, or false when it is driving nothing.
	Live(id string) (AirPlayControl, bool)
}
