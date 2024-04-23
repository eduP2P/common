package key

import (
	crand "crypto/rand"
	"fmt"
	"io"
)

// rand fills b with cryptographically strong random bytes. Panics if
// no random bytes are available.
func rand(b []byte) {
	if _, err := io.ReadFull(crand.Reader, b[:]); err != nil {
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
