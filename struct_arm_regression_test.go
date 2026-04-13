//go:build arm && (freebsd || linux || netbsd || windows)

package purego

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestARMAddStructCopiesUnaddressableValue(t *testing.T) {
	type sample struct{ X uint32 }
	var numInts, numFloats, numStack int
	var ptr uintptr
	keepAlive := addStruct(reflect.ValueOf(sample{X: 7}), &numInts, &numFloats, &numStack, func(v uintptr) {
		ptr = v
	}, func(uintptr) {
		t.Fatal("unexpected float register")
	}, func(uintptr) {
		t.Fatal("unexpected stack slot")
	}, nil)
	if len(keepAlive) == 0 {
		t.Fatal("expected copied keepAlive value")
	}
	if got := *(*uint32)(unsafe.Pointer(ptr)); got != 7 {
		t.Fatalf("copied value = %#x, want %#x", got, uint32(7))
	}
}

func TestARMPlaceRegistersReadsUnaddressableValue(t *testing.T) {
	type sample struct{ X uint32 }
	var ints []uintptr
	placeRegisters(reflect.ValueOf(sample{X: 7}), func(uintptr) {
		t.Fatal("unexpected float register")
	}, func(v uintptr) {
		ints = append(ints, v)
	})
	if len(ints) != 1 || ints[0] != 7 {
		t.Fatalf("placeRegisters() = %#v, want [7]", ints)
	}
}
