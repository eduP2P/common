package msgcontrol

type ControlMessage interface {
	CMsgType() ControlMessageType
}

func (c *ClientHello) CMsgType() ControlMessageType {
	return ClientHelloType
}
func (c *ServerHello) CMsgType() ControlMessageType {
	return ServerHelloType
}
func (c *Logon) CMsgType() ControlMessageType {
	return LogonType
}
func (c *LogonAuthenticate) CMsgType() ControlMessageType {
	return LogonAuthenticateType
}
func (c *LogonDeviceKey) CMsgType() ControlMessageType {
	return LogonDeviceKeyType
}
func (c *LogonAccept) CMsgType() ControlMessageType {
	return LogonAcceptType
}
func (c *LogonReject) CMsgType() ControlMessageType {
	return LogonRejectType
}
func (c *Logout) CMsgType() ControlMessageType {
	return LogoutType
}
func (c *Ping) CMsgType() ControlMessageType {
	return PingType
}
func (c *Pong) CMsgType() ControlMessageType {
	return PongType
}

func (c *EndpointUpdate) CMsgType() ControlMessageType {
	return EndpointUpdateType
}
func (c *HomeRelayUpdate) CMsgType() ControlMessageType {
	return HomeRelayUpdateType
}
func (c *PeerAddition) CMsgType() ControlMessageType {
	return PeerAdditionType
}
func (c *PeerUpdate) CMsgType() ControlMessageType {
	return PeerUpdateType
}
func (c *PeerRemove) CMsgType() ControlMessageType {
	return PeerRemoveType
}
func (c *RelayUpdate) CMsgType() ControlMessageType {
	return RelayUpdateType
}
