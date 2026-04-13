//go:build s390x && linux

package purego

import (
	"math"
	"reflect"
	"testing"
)

func TestS390xGetStructUsesHighFloat32Bits(t *testing.T) {
	type pair struct{ X, Y float32 }
	sys := syscall15Args{
		f1: uintptr(uint64(math.Float32bits(1.5)) << 32),
		f2: uintptr(uint64(math.Float32bits(2.5)) << 32),
	}
	got := getStruct(reflect.TypeOf(pair{}), sys).Interface().(pair)
	if got.X != 1.5 || got.Y != 2.5 {
		t.Fatalf("got %+v, want {1.5 2.5}", got)
	}
}
