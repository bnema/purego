//go:build amd64 && (darwin || linux)

package purego

import (
	"reflect"
	"testing"
)

func TestGetStructHandlesSecondEightbyteWithoutOffset8Field(t *testing.T) {
	type awkward struct {
		A [3]uint32
		B uint32
	}
	sys := syscall15Args{a1: 0x0000000200000001, a2: 0x0000000400000003}
	got := getStruct(reflect.TypeOf(awkward{}), sys).Interface().(awkward)
	want := awkward{A: [3]uint32{1, 2, 3}, B: 4}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
