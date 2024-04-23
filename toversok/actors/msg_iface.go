package actors

// This file contains the ActorMessage interface, and dud bindings

type ActorMessage interface {
	amsg()
}

func (o *TManConnActivity) amsg()               {}
func (o *TManConnGoodBye) amsg()                {}
func (o *TManSessionMessageFromRelay) amsg()    {}
func (o *TManSessionMessageFromDirect) amsg()   {}
func (o *TManSetPeerInfo) amsg()                {}
func (o *SManSessionFrameFromRelay) amsg()      {}
func (o *SManSessionFrameFromAddrPort) amsg()   {}
func (o *SManSendSessionMessageToDirect) amsg() {}
func (o *SManSendSessionMessageToRelay) amsg()  {}
func (o *OutConnUse) amsg()                     {}
func (o *RManUpdateRelayConfiguration) amsg()   {}
func (o *DManSetMTU) amsg()                     {}
func (o *DRouterPeerClearKnownAs) amsg()        {}
func (o *DRouterPeerAddKnownAs) amsg()          {}
