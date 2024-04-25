package stun

// Request generates a binding request STUN packet.
// The transaction ID, tID, should be a random sequence of bytes.
func Request(tID TxID) []byte {
	// STUN header, RFC5389 Section 6.
	const lenAttrSoftware = 4 + len(thisSoftware)
	b := make([]byte, 0, headerLen+lenAttrSoftware+lenFingerprint)
	b = append(b, bindingRequest...)
	b = appendU16(b, uint16(lenAttrSoftware+lenFingerprint)) // number of bytes following header
	b = append(b, magicCookie...)
	b = append(b, tID[:]...)

	// Attribute SOFTWARE, RFC5389 Section 15.5.
	b = appendU16(b, attrNumSoftware)
	b = appendU16(b, uint16(len(thisSoftware)))
	b = append(b, thisSoftware...)

	// Attribute FINGERPRINT, RFC5389 Section 15.5.
	fp := fingerPrint(b)
	b = appendU16(b, attrNumFingerprint)
	b = appendU16(b, 4)
	b = appendU32(b, fp)

	return b
}
