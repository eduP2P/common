package msgcontrol

//type BSONAddrPort netip.AddrPort
//
//func (a *BSONAddrPort) MarshalBSONValue() (bsontype.Type, []byte, error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (a *BSONAddrPort) UnmarshalBSONValue(b bsontype.Type, bytes []byte) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//type BSONAddr netip.Addr
//
//func (a *BSONAddr) MarshalBSONValue() (bsontype.Type, []byte, error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (a *BSONAddr) UnmarshalBSONValue(b bsontype.Type, bytes []byte) error {
//	//TODO implement me
//	panic("implement me")
//}

//func ToBSONAddrPairArray(endpoints []netip.AddrPort) []BSONAddrPort {
//	converted := make([]BSONAddrPort, len(endpoints))
//	for i, e := range endpoints {
//		converted[i] = BSONAddrPort(e)
//	}
//	return converted
//}
//
//func FromBSONAddrPairArray(endpoints []BSONAddrPort) []netip.AddrPort {
//	converted := make([]netip.AddrPort, len(endpoints))
//	for i, e := range endpoints {
//		converted[i] = netip.AddrPort(e)
//	}
//	return converted
//}
