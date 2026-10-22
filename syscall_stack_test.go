// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build !windows && !plan9

package purego_test

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego/internal/load"
)

//go:noinline
func growCallbackStack(depth int) {
	var pad [4096]byte
	pad[0] = byte(depth)
	if depth > 0 {
		growCallbackStack(depth - 1)
	}
	runtime.KeepAlive(pad)
}

func TestSyscall6CallbackStackRelocation(t *testing.T) {
	library, err := getSystemLibrary()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := load.OpenLibrary(library)
	if err != nil {
		t.Fatal(err)
	}
	defer load.CloseLibrary(lib)
	search, err := load.OpenSymbol(lib, "bsearch")
	if err != nil {
		t.Fatal(err)
	}
	for _, fast := range []bool{false, true} {
		name := "NewCallback"
		if fast {
			name = "NewCallbackInts"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			compare := func() uintptr { calls++; growCallbackStack(128); return 1 }
			var cb uintptr
			if fast {
				cb = purego.NewCallbackInts(func(*purego.CallbackArgs) uintptr { return compare() })
			} else {
				cb = purego.NewCallback(func(_ purego.CDecl, _, _ uintptr) uintptr { return compare() })
			}
			// A fresh goroutine starts with a small stack for each callback kind.
			done := make(chan uintptr, 1)
			go func() {
				key, data := new(uintptr), new(uintptr)
				r, _, _ := purego.Syscall6(search, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(data)), 1, unsafe.Sizeof(*data), cb, 0)
				done <- r
			}()
			if got := <-done; got != 0 {
				t.Errorf("bsearch returned %#x, want NULL (key absent)", got)
			}
			if calls != 1 {
				t.Errorf("comparator called %d times, want 1", calls)
			}
		})
	}
}

func TestSyscall6Allocs(t *testing.T) {
	library, err := getSystemLibrary()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := load.OpenLibrary(library)
	if err != nil {
		t.Fatal(err)
	}
	defer load.CloseLibrary(lib)
	fn, err := load.OpenSymbol(lib, "abs")
	if err != nil {
		t.Fatal(err)
	}
	if got := testing.AllocsPerRun(100, func() { purego.Syscall6(fn, 1, 0, 0, 0, 0, 0) }); got != 0 {
		t.Fatalf("Syscall6 allocations = %v, want 0", got)
	}
}
