package key

import (
	"encoding"
)

type key interface {
	IsZero() bool
}

type canTextMarshal interface {
	// We need text encoding for JSON and BSON (currently)

	encoding.TextMarshaler
	encoding.TextUnmarshaler

	// TODO maybe also allow/support BSON marshalling
	//bson.ValueMarshaler
	//bson.Unmarshaler

	// TODO maybe also allow/support binary marshalling
	// encoding.BinaryMarshaler
	// encoding.BinaryUnmarshaler
}

//type canBsonMarshal interface {
//	bson.ValueMarshaler
//	bson.ValueUnmarshaler
//
//	// TODO maybe also allow/support binary marshalling
//	// encoding.BinaryMarshaler
//	// encoding.BinaryUnmarshaler
//}

type publicKey interface {
	key

	IsZero() bool
	Debug() string
	HexString() string
	// TODO
}

type privateKey[Pub key] interface {
	key

	Public() Pub

	// TODO
}

type canSealTo[To publicKey] interface {
	SealTo(p To, cleartext []byte) (ciphertext []byte)
}

type canOpenFrom[From publicKey] interface {
	OpenFrom(p From, ciphertext []byte) (cleartext []byte, ok bool)
}

type CryptoPair[Pub publicKey] interface {
	canOpenFrom[Pub]
	canSealTo[Pub]
}

type createSharedKey[Pub publicKey, Priv privateKey[Pub], Shared sharedKey[Pub, Priv]] interface {
	Shared(Pub) Shared
}

type sharedKey[Pub publicKey, Priv privateKey[Pub]] interface {
	key

	Seal(cleartext []byte) (ciphertext []byte)

	Open(ciphertext []byte) (cleartext []byte, ok bool)
}
