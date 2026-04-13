//go:build (386 || arm) && (freebsd || linux || netbsd || windows)

package purego

import "testing"

func TestSyscall15ArgsSetDoesNotPanicWithMaxArgs(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Set panicked: %v", r)
		}
	}()

	var s syscall15Args
	s.Set(1, make([]uintptr, maxArgs), make([]uintptr, maxArgs), 8)
}
