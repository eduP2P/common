package actors

type EndpointManager struct {
	*ActorCommon
	s *Stage
}

// TODO
//  - receive relay info
//  - do STUN requests to each and resolve remote endpoints
//    - maybe determine when symmetric nat / "varies" is happening
//  - do latency determination
//    - inform relay manager that results are ready
//    - relay manager switches home relay and informs stage of that decision
//  - collect local endpoints

// TODO future:
//  - UPnP? Other stuff?

func (em *EndpointManager) Run() {
	// TODO
	panic("todo")
}

func (em *EndpointManager) Close() {
	//TODO implement me
	panic("implement me")
}
