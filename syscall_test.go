// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 The Ebitengine Authors

package purego_test

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego/internal/load"
)

func TestOS(t *testing.T) {
	// set and unset an environment variable since this calls into fakecgo.
	err := os.Setenv("TESTING", "SOMETHING")
	if err != nil {
		t.Errorf("failed to Setenv: %s", err)
	}
	err = os.Unsetenv("TESTING")
	if err != nil {
		t.Errorf("failed to Unsetenv: %s", err)
	}
}

func TestErrno(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("platform does not support returning errno from syscall")
	}

	libc, err := load.OpenLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		t.Fatal(err)
	}

	openSym, err := load.OpenSymbol(libc, "open")
	if err != nil {
		t.Fatal(err)
	}

	r1, _, errno := purego.SyscallN(openSym, uintptr(unsafe.Pointer(&[]byte("_file_that_does_not_exist_\x00")[0])), uintptr(os.O_RDWR))
	if int32(r1) != -1 {
		t.Errorf("open returned %d, wanted -1", r1)
	}

	var strerror func(int32) string
	purego.RegisterLibFunc(&strerror, libc, "strerror")

	const expected = "No such file or directory"
	got := strerror(int32(errno))
	if got != expected {
		t.Errorf("strerror returned %q, wanted \"%s\"", got, expected)
	}
}

func TestSyscallSelf(t *testing.T) {
	// Test that SyscallSelf(fn, self, args...) produces the same result as
	// SyscallN(fn, self, args...) for various argument counts.

	// sum2 takes (self, a1) and returns self + a1
	sum2 := purego.NewCallback(func(self, a1 uintptr) uintptr {
		return self + a1
	})
	// sum4 takes (self, a1, a2, a3) and returns self + a1 + a2 + a3
	sum4 := purego.NewCallback(func(self, a1, a2, a3 uintptr) uintptr {
		return self + a1 + a2 + a3
	})

	tests := []struct {
		name string
		fn   uintptr
		self uintptr
		args []uintptr
		want uintptr
	}{
		{"no extra args", sum2, 10, []uintptr{20}, 30},
		{"zero self", sum2, 0, []uintptr{42}, 42},
		{"multiple args", sum4, 1, []uintptr{2, 3, 4}, 10},
		{"no variadic args",
			purego.NewCallback(func(self uintptr) uintptr { return self * 3 }),
			7, nil, 21},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r1, _, _ := purego.SyscallSelf(tc.fn, tc.self, tc.args...)
			if r1 != tc.want {
				t.Errorf("SyscallSelf returned %d, want %d", r1, tc.want)
			}

			// Verify it matches SyscallN with manually prepended self
			allArgs := make([]uintptr, 0, 1+len(tc.args))
			allArgs = append(allArgs, tc.self)
			allArgs = append(allArgs, tc.args...)
			r1n, _, _ := purego.SyscallN(tc.fn, allArgs...)
			if r1 != r1n {
				t.Errorf("SyscallSelf result %d differs from SyscallN result %d", r1, r1n)
			}
		})
	}
}

func TestSyscallSelfPanics(t *testing.T) {
	t.Run("nil fn", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil fn")
			}
		}()
		purego.SyscallSelf(0, 1)
	})

	t.Run("too many args", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic for too many args")
			}
			msg := fmt.Sprint(r)
			if msg != "purego: too many arguments to SyscallSelf" {
				t.Errorf("unexpected panic message: %s", msg)
			}
		}()
		// Create a valid callback so we don't panic on nil fn
		fn := purego.NewCallback(func(uintptr) uintptr { return 0 })
		// Pass more args than maxArgs-1 allows (maxArgs is 15 on 64-bit)
		args := make([]uintptr, 15)
		purego.SyscallSelf(fn, 1, args...)
	})
}
