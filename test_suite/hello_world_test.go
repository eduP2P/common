package hello_world

import (
	"testing"
)

func TestHelloWorld(t *testing.T) {
	ret := hello_world()

	if ret != 0 {
		t.Error("Incorrect return value")
	}
}
