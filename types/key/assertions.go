package key

// NODE

var (
	_ publicKey = NodePublic{}

	_ privateKey[NodePublic] = NodePrivate{}

	// We need this to send keys over the wire via JSON
	_ canTextMarshal = &NodePublic{}

	// We need this to persist node keys to disk.
	_ canTextMarshal = &NodePrivate{}

	_ CryptoPair[NodePublic] = NodePrivate{}
)

// SESSION

var (
	_ publicKey = SessionPublic{}

	// We need this to send keys over the wire
	_ canTextMarshal = &SessionPublic{}

	_ privateKey[SessionPublic] = SessionPrivate{}

	_ createSharedKey[SessionPublic, SessionPrivate, SessionShared] = SessionPrivate{}

	// Redundant by createSharedKey, but just to be sure
	_ sharedKey[SessionPublic, SessionPrivate] = SessionShared{}
)

// CONTROL

var (
	_ publicKey = ControlPublic{}

	_ privateKey[ControlPublic] = ControlPrivate{}

	// We need this to send keys over the wire
	_ canTextMarshal = &ControlPublic{}

	// We need this to save private control keys
	_ canTextMarshal = &ControlPrivate{}

	// Furthermore, there needs to be
	//  - OpenFromNode
	//  - OpenFromSession
	//  - SealToNode
)
