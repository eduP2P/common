package key

// NakedKey is the 32-byte underlying key.
//
// Only ever used for public interfaces, very dangerous to use directly, due to the security implications.
type NakedKey [32]byte
