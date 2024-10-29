package key

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"go4.org/mem"
	"golang.org/x/crypto/nacl/box"
	"io"
	"slices"
)

// rand fills b with cryptographically strong random bytes. Panics if
// no random bytes are available.
func rand(b []byte) {
	if _, err := io.ReadFull(crand.Reader, b); err != nil {
		panic(fmt.Sprintf("unable to read random bytes from OS: %v", err))
	}
}

// clamp25519 clamps b, which must be a 32-byte Curve25519 private
// key, to a safe value.
//
// The clamping effectively constrains the key to a number between
// 2^251 and 2^252-1, which is then multiplied by 8 (the cofactor of
// Curve25519). This produces a value that doesn't have any unsafe
// properties when doing operations like ScalarMult.
//
// See
// https://web.archive.org/web/20210228105330/https://neilmadden.blog/2020/05/28/whats-the-curve25519-clamping-all-about/
// for a more in-depth explanation of the constraints that led to this
// clamping requirement.
//
// PLEASE NOTE that not all Curve25519 values require clamping. When
// implementing a new key type that uses Curve25519, you must evaluate
// whether that particular key's use requires clamping. Here are some
// existing uses and whether you should clamp private keys at
// creation.
//
//   - NaCl box: yes, clamp at creation.
//   - WireGuard (userspace uapi or kernel): no, do not clamp.
//   - Noise protocols: no, do not clamp.
//
// (Taken from tailscale)
func clamp25519Private(b []byte) {
	b[0] &= 248
	b[31] = (b[31] & 127) | 64
}

func appendHexKey(dst []byte, prefix string, key []byte) []byte {
	dst = slices.Grow(dst, len(prefix)+hex.EncodedLen(len(key)))
	dst = append(dst, prefix...)
	dst = hex.AppendEncode(dst, key)
	return dst
}

// parseHex decodes a key string of the form "<prefix><hex string>"
// into out. The prefix must match, and the decoded base64 must fit
// exactly into out.
//
// Note the errors in this function deliberately do not echo the
// contents of in, because it might be a private key or part of a
// private key.
func parseHex(out []byte, in, prefix mem.RO) error {
	if !mem.HasPrefix(in, prefix) {
		return fmt.Errorf("key hex string doesn't have expected type prefix %s", prefix.StringCopy())
	}
	in = in.SliceFrom(prefix.Len())
	if want := len(out) * 2; in.Len() != want {
		return fmt.Errorf("key hex has the wrong size, got %d want %d", in.Len(), want)
	}
	for i := range out {
		a, ok1 := fromHexChar(in.At(i*2 + 0))
		b, ok2 := fromHexChar(in.At(i*2 + 1))
		if !ok1 || !ok2 {
			return errors.New("invalid hex character in key")
		}
		out[i] = (a << 4) | b
	}
	return nil
}

// fromHexChar converts a hex character into its value and a success flag.
func fromHexChar(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}

	return 0, false
}

// Assumed are that priv and pub are not zero
func openFrom(priv, pub NakedKey, ciphertext []byte) (cleartext []byte, ok bool) {
	if len(ciphertext) < 24 {
		return nil, false
	}
	nonce := (*[24]byte)(ciphertext)
	return box.Open(nil, ciphertext[len(nonce):], nonce, (*[32]byte)(&pub), (*[32]byte)(&priv))
}

// Assumed are that priv and pub are not zero
func sealTo(priv, pub NakedKey, cleartext []byte) (ciphertext []byte) {
	var nonce [24]byte
	rand(nonce[:])
	return box.Seal(nonce[:], cleartext, &nonce, (*[32]byte)(&pub), (*[32]byte)(&priv))
}
