package airplay

// Audio encryption for receivers that ask for it (et=1).
//
// The scheme is AES-128-CBC over the RTP payload, with the key handed to the
// receiver in the ANNOUNCE: wrapped with RSA-OAEP under a public key that is
// the same for every AirPlay receiver in the world. It is published — it was
// extracted from an AirPort Express firmware image well over a decade ago and
// appears in every open-source sender — and it protects nothing: any sender
// can wrap a key with it, and the "authentication" it provides is a licensing
// formality rather than a security property.
//
// Which is why Device.Cipher prefers cleartext when the receiver offers it.
// This path exists for the receivers that don't.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
)

// appleModulus is the AirPort Express RSA public modulus, base64 as it appears
// in every implementation of this protocol. Public by construction: it is the
// key senders encrypt *to*.
const appleModulus = "59dE8qLieItsH1WgjrcFRKj6eUWqi+bGLOX1HL3U3GhC/j0Qg90u3sG/1CUtwC5v" +
	"OYvfDmFI6oSFXi5ELabWJmT2dKHzBJKa3k9ok+8t9ucRqMd6DZHJ2YCCLlDRKSKv" +
	"6kDqnw4UwPdpOMXziC/AMj3Z/lUVX1G7WSHCAWKf1zNS1eLvqr+boEjXuBOitnZ/" +
	"bDzPHrTOZz0Dew0uowxf/+sG+NCK3eQJVxqcaJ/vEHKIVd2M+5qL71yJQ+87X6oV" +
	"3eaYvt3zWZYD6z5vYTcrtij2VZ9Zmni/UAaHqn9JdsBWLUEpVviYnhimNVvYFZeC" +
	"Xg/IdTQ+x4IRdiXNv5hEew=="

// appleExponent is 65537, the exponent that accompanies it.
const appleExponent = 65537

// cipherKey is a session's audio key: the AES key and IV, plus the wrapped
// form that goes in the SDP.
type cipherKey struct {
	block cipher.Block
	iv    []byte
	// WrappedKey and IV are base64 for the a=rsaaeskey / a=aesiv lines.
	WrappedKey string
	IVBase64   string
}

// newCipherKey mints a random AES-128 key and IV and wraps the key for the
// receiver.
func newCipherKey() (*cipherKey, error) {
	key := make([]byte, 16)
	iv := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("airplay: generating audio key: %w", err)
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("airplay: generating audio iv: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	wrapped, err := wrapKey(key)
	if err != nil {
		return nil, err
	}
	return &cipherKey{
		block: block,
		iv:    iv,
		// Padding is stripped: the SDP attribute is written without it,
		// and receivers that re-pad before decoding expect that.
		WrappedKey: trimBase64Pad(base64.StdEncoding.EncodeToString(wrapped)),
		IVBase64:   trimBase64Pad(base64.StdEncoding.EncodeToString(iv)),
	}, nil
}

// wrapKey RSA-OAEP-encrypts the AES key under Apple's public key.
func wrapKey(key []byte) ([]byte, error) {
	mod, err := base64.StdEncoding.DecodeString(appleModulus)
	if err != nil {
		return nil, fmt.Errorf("airplay: decoding receiver key: %w", err)
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(mod), E: appleExponent}
	return rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, key, nil)
}

// encrypt enciphers a payload in place-compatible fashion: AES-CBC over whole
// 16-byte blocks, with any trailing remainder left in the clear.
//
// That trailing plaintext looks like a mistake and is not — it is what the
// protocol specifies, and a receiver decrypting a stream would produce noise
// at the end of every packet if the sender padded instead. Each packet starts
// from the session IV rather than chaining across packets, because packets
// are UDP and may be lost, and a chained stream could not survive that.
func (k *cipherKey) encrypt(payload []byte) []byte {
	if k == nil {
		return payload
	}
	n := len(payload) / aes.BlockSize * aes.BlockSize
	if n == 0 {
		return payload
	}
	out := make([]byte, len(payload))
	copy(out, payload)
	cipher.NewCBCEncrypter(k.block, k.iv).CryptBlocks(out[:n], out[:n])
	return out
}

// trimBase64Pad drops '=' padding, which SDP attributes in this protocol are
// written without.
func trimBase64Pad(s string) string {
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return s
}

// ntpNow is the current time as a 64-bit NTP timestamp: seconds since 1900 in
// the high half, binary fraction in the low half. This is the clock every
// receiver in a cast disciplines itself to, so it is read once per sync packet
// from one place rather than per receiver.
func ntpTime(unixNanos int64) uint64 {
	const epochOffset = 2208988800 // seconds between 1900-01-01 and 1970-01-01
	secs := uint64(unixNanos/1e9) + epochOffset
	frac := uint64(unixNanos%1e9) << 32 / 1e9
	return secs<<32 | frac
}

// putNTP writes an NTP timestamp big-endian.
func putNTP(b []byte, t uint64) { binary.BigEndian.PutUint64(b, t) }
