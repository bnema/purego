// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build !windows && !plan9

package purego_test

import (
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego/internal/load"
)

//go:noinline
func writeDeep(sym uintptr, depth int) bool {
	var pad [1024]byte
	pad[0] = byte(depth)
	if depth > 0 {
		ok := writeDeep(sym, depth-1)
		runtime.KeepAlive(pad)
		return ok
	}
	var local uint64
	purego.Syscall6(sym, uintptr(unsafe.Pointer(&local)), 42, 0, 0, 0, 0)
	runtime.KeepAlive(&local)
	runtime.KeepAlive(pad)
	return local == 42
}

func TestSyscall6PointerLiveness(t *testing.T) {
	libFile := filepath.Join(t.TempDir(), "libbenchmark.so")
	if err := buildSharedLib(t, "CC", libFile, filepath.Join("testdata", "benchmarktest", "benchmark.c")); err != nil {
		t.Fatal(err)
	}
	lib, err := load.OpenLibrary(libFile)
	if err != nil {
		t.Fatal(err)
	}
	defer load.CloseLibrary(lib)
	sym, err := load.OpenSymbol(lib, "write_through")
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				runtime.GC()
				debug.FreeOSMemory()
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()

	for i := 0; i < 32; i++ {
		if !writeDeep(sym, 0) {
			t.Fatal("stack local not updated")
		}
	}
	if !writeDeep(sym, 64) {
		t.Fatal("deep stack local not updated")
	}
	heap := new(uint64)
	purego.Syscall6(sym, uintptr(unsafe.Pointer(heap)), 42, 0, 0, 0, 0)
	runtime.KeepAlive(heap)
	if *heap != 42 {
		t.Fatalf("heap value = %d", *heap)
	}
}
