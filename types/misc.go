package types

// Contains miscellaneous functions and types

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"golang.org/x/exp/maps"
)

// Incomparable is a zero-width incomparable type. If added as the
// first field in a struct, it marks that struct as not comparable
// (can't do == or be a map key) and usually doesn't add any width to
// the struct (unless the struct has only small fields).
//
// Be making a struct incomparable, you can prevent misuse (prevent
// people from using ==), but also you can shrink generated binaries,
// as the compiler can omit equality funcs from the binary.
//
// (Taken from the tailscale types library)
type Incomparable [0]func()

// SetSubtraction returns the elements in `a` that aren't in `b`.
//
// in set notation: a - b
func SetSubtraction[T comparable](a, b []T) []T {
	set := make(map[T]interface{})

	for _, x := range a {
		set[x] = struct{}{}
	}
	for _, x := range b {
		delete(set, x)
	}

	return maps.Keys(set)
}

// SetUnion returns a set of elements that were either in a and b
// in set notation: a u b
func SetUnion[T comparable](a, b []T) []T {
	set := make(map[T]interface{})

	for _, x := range a {
		set[x] = nil
	}
	for _, x := range b {
		set[x] = nil
	}

	return maps.Keys(set)
}

func PtrOr[T any](v *T, def T) T {
	if v == nil {
		return def
	} else {
		return *v
	}
}

func SliceOrEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	} else {
		return v
	}
}

func SliceOrNil[T any](v []T) []T {
	if v == nil || (v != nil && len(v) > 0) {
		return v
	} else {
		// len(v) == 0
		return nil
	}
}

// IsContextDone does a quick check on a context to see if its dead.
func IsContextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// RandStringBytesMaskImprSrc returns a random hexadecimal string of length n.
func RandStringBytesMaskImprSrc(n int) string {
	b := make([]byte, (n+1)/2) // can be simplified to n/2 if n is always even

	if _, err := rand.Read(b); err != nil {
		panic(err)
	}

	return hex.EncodeToString(b)[:n]
}
