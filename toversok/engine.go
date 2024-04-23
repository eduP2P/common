package toversok

type Engine struct {
}

func (e *Engine) Start() error {
	// TODO
	return nil
}

func NewEngine() Engine {
	// TODO
	return Engine{}
}

func (e *Engine) Handle(ev Event) error {
	switch ev.(type) {
	case RelayUpdate:
		// TODO
		break
	case PeerAddition:
		// TODO
		break
	case PeerRemoval:
		// TODO
		break
	case PeerUpdate:
		// TODO
		break
	default:
		// TODO warn-log about unknown type instead of panic
		panic("Unknown type!")
	}

	// TODO
	panic("implement me")
}

// handleglobalstate
// global peer_state includes:
// - relay map of ID -> relay address + settings

// handlepeerstate
// peer peer_state includes:
// - home relay ID
// - public key
// - virtual IP
