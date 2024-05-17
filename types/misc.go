package types

import "golang.org/x/exp/maps"

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
