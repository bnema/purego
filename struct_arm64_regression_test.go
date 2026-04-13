//go:build arm64 && (darwin || linux)

package purego

import (
	"reflect"
	"testing"
)

func TestIsHFARejectsNestedMixedStruct(t *testing.T) {
	type inner struct{ X, Y float32 }
	type mixed struct {
		Inner inner
		Tail  int32
	}
	if isHFA(reflect.TypeOf(mixed{})) {
		t.Fatal("expected mixed nested struct to be non-HFA")
	}
}

func TestIsHFAAcceptsNestedFloatStructs(t *testing.T) {
	type inner struct{ X, Y float32 }
	type outer struct {
		Left  inner
		Right inner
	}
	if !isHFA(reflect.TypeOf(outer{})) {
		t.Fatal("expected nested float-only struct to be HFA")
	}
}

func TestIsHFAAcceptsFloatArrayStruct(t *testing.T) {
	type arrayed struct{ A [2]float64 }
	if !isHFA(reflect.TypeOf(arrayed{})) {
		t.Fatal("expected float array struct to be HFA")
	}
}
