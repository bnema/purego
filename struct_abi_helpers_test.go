package purego

import (
	"math"
	"reflect"
	"testing"
)

func TestPackFloat32PairMasksLowWord(t *testing.T) {
	low := uintptr(0xDEADBEEF00000000 | uint64(math.Float32bits(1.5)))
	high := uintptr(math.Float32bits(2.5))
	want := uintptr(uint64(math.Float32bits(2.5))<<32 | uint64(math.Float32bits(1.5)))
	if got := packFloat32Pair(low, high); got != want {
		t.Fatalf("packFloat32Pair() = %#x, want %#x", got, want)
	}
}

func TestHighFloat32Bits(t *testing.T) {
	word := uintptr(uint64(math.Float32bits(3.25)) << 32)
	if got, want := highFloat32Bits(word), math.Float32bits(3.25); got != want {
		t.Fatalf("highFloat32Bits() = %#x, want %#x", got, want)
	}
}

func TestFlushPackedWordResetsState(t *testing.T) {
	var val uint64 = 0x11223344
	shift := byte(32)
	flushed := false
	var gotFloat uintptr
	flushPackedWord(&val, &shift, &flushed, true, func(v uintptr) { gotFloat = v }, func(uintptr) {
		t.Fatal("unexpected integer flush")
	})
	if !flushed || val != 0 || shift != 0 {
		t.Fatalf("flushPackedWord() left state dirty: flushed=%v val=%#x shift=%d", flushed, val, shift)
	}
	if gotFloat != 0x11223344 {
		t.Fatalf("flushPackedWord() flushed %#x, want %#x", gotFloat, uintptr(0x11223344))
	}
}

func TestStableValuePointerCopiesUnaddressableValue(t *testing.T) {
	type sample struct{ X uint32 }
	ptr, keepAlive := stableValuePointer(reflect.ValueOf(sample{X: 7}))
	if keepAlive == nil {
		t.Fatal("expected keepAlive copy for unaddressable value")
	}
	if got := *(*uint32)(ptr); got != 7 {
		t.Fatalf("stableValuePointer() copied %#x, want %#x", got, uint32(7))
	}
}

func TestIsAllSameFloatRejectsNestedMixedStruct(t *testing.T) {
	type inner struct{ X, Y float32 }
	type mixed struct {
		Inner inner
		Tail  int32
	}
	allFloats, fields := isAllSameFloat(reflect.TypeOf(mixed{}))
	if allFloats {
		t.Fatalf("isAllSameFloat() = true for mixed nested struct with %d fields", fields)
	}
}

func TestIsAllSameFloatAcceptsNestedFloatStruct(t *testing.T) {
	type inner struct{ X, Y float32 }
	type outer struct {
		Left  inner
		Right inner
	}
	allFloats, fields := isAllSameFloat(reflect.TypeOf(outer{}))
	if !allFloats || fields != 4 {
		t.Fatalf("isAllSameFloat() = (%v, %d), want (true, 4)", allFloats, fields)
	}
}
