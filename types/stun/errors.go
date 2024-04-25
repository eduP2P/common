package stun

import "errors"

var (
	ErrNotSTUN            = errors.New("response is not a STUN packet")
	ErrNotSuccessResponse = errors.New("STUN packet is not a response")
	ErrMalformedAttrs     = errors.New("STUN response has malformed attributes")
	ErrNotBindingRequest  = errors.New("STUN request not a binding request")
	ErrWrongSoftware      = errors.New("STUN request came from non-Tailscale software")
	ErrNoFingerprint      = errors.New("STUN request didn't end in fingerprint")
	ErrWrongFingerprint   = errors.New("STUN request had bogus fingerprint")
)
