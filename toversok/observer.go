package toversok

// Observer functions as a state observer for the Engine, effectively allowing calling clients to peek into the engine state in an abstracted way.
type Observer interface {
	RegisterStateChangeListener(func(state EngineState))

	CurrentState() EngineState

	GetNeedsLoginState() (url string, err error)
	GetEstablishedState() // TODO
}

type EngineState byte

//go:generate go run golang.org/x/tools/cmd/stringer -type=EngineState
const (
	NoSession EngineState = iota
	CreatingSession
	NeedsLogin
	Established
	StoppingSession
)
