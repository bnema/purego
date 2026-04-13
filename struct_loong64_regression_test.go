//go:build loong64 && linux

package purego

import (
	"math"
	"reflect"
	"testing"
)

func TestLoong64GetStructMasksFloat32Pairs(t *testing.T) {
	type pair struct{ X, Y float32 }
	sys := syscall15Args{
		f1: uintptr(0xDEADBEEF00000000 | uint64(math.Float32bits(1.5))),
		f2: uintptr(math.Float32bits(2.5)),
	}
	got := getStruct(reflect.TypeOf(pair{}), sys).Interface().(pair)
	if got.X != 1.5 || got.Y != 2.5 {
		t.Fatalf("got %+v, want {1.5 2.5}", got)
	}
}

func TestLoong64PlaceRegistersResetsAfterFlush(t *testing.T) {
	type triple struct{ A, B, C uint32 }
	var ints []uintptr
	placeRegisters(reflect.ValueOf(triple{A: 1, B: 2, C: 3}), func(uintptr) {
		t.Fatal("unexpected float register")
	}, func(v uintptr) {
		ints = append(ints, v)
	})
	want := []uintptr{0x0000000200000001, 0x0000000000000003}
	if len(ints) != len(want) || ints[0] != want[0] || ints[1] != want[1] {
		t.Fatalf("placeRegisters() = %#v, want %#v", ints, want)
	}
}
