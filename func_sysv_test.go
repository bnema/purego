// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

//go:build darwin || freebsd || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || (cgo && s390x))) || netbsd

package purego_test

import (
	"testing"

	"github.com/bnema/purego"
)

func TestUnrefCallback(t *testing.T) {
	imp := func() bool { return true }

	if err := purego.UnrefCallback(0); err == nil {
		t.Fatal("unref of unknown callback returned nil error")
	}

	ref := purego.NewCallback(imp)
	if err := purego.UnrefCallback(ref); err != nil {
		t.Fatalf("UnrefCallback(%#x) error = %v", ref, err)
	}
	if err := purego.UnrefCallback(ref); err == nil {
		t.Fatal("second UnrefCallback returned nil error")
	}
}

func TestNewCallbackFnPtrReuseAndUnref(t *testing.T) {
	imp := func() bool { return true }

	if err := purego.UnrefCallbackFnPtr(&imp); err == nil {
		t.Fatal("unref of unknown callback function pointer returned nil error")
	}

	ref1 := purego.NewCallbackFnPtr(&imp)
	ref2 := purego.NewCallbackFnPtr(&imp)
	if ref1 != ref2 {
		t.Fatalf("NewCallbackFnPtr did not reuse callback: got %#x and %#x", ref1, ref2)
	}

	if err := purego.UnrefCallbackFnPtr(&imp); err != nil {
		t.Fatalf("UnrefCallbackFnPtr error = %v", err)
	}
	if err := purego.UnrefCallbackFnPtr(&imp); err == nil {
		t.Fatal("second UnrefCallbackFnPtr returned nil error")
	}
	if err := purego.UnrefCallback(ref1); err == nil {
		t.Fatal("UnrefCallback on released function-pointer callback returned nil error")
	}
}
