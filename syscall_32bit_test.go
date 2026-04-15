//go:build (386 || arm) && (freebsd || linux || netbsd || windows)

package purego_test

import (
	"fmt"
	"testing"

	"github.com/bnema/purego"
)

func TestSyscallN32BitPanicsAt16Args(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if got := fmt.Sprint(r); got != "purego: too many arguments to SyscallN" {
			t.Fatalf("panic = %q, want %q", got, "purego: too many arguments to SyscallN")
		}
	}()
	purego.SyscallN(uintptr(1), make([]uintptr, 16)...)
}

func TestSyscallSelf32BitPanicsAt16Args(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if got := fmt.Sprint(r); got != "purego: too many arguments to SyscallSelf" {
			t.Fatalf("panic = %q, want %q", got, "purego: too many arguments to SyscallSelf")
		}
	}()
	purego.SyscallSelf(uintptr(1), 1, make([]uintptr, 15)...)
}
