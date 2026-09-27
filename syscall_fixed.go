// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build darwin || freebsd || linux || netbsd || windows

package purego

import (
	"runtime"
	"unsafe"
)

//go:noescape
//go:linkname noescapeFixed runtime.noescape
func noescapeFixed(p unsafe.Pointer) unsafe.Pointer

// Syscall15 calls fn with up to 15 integer-class arguments without
// allocating. Unused arguments must be 0. Unlike SyscallN it does not use a
// variadic argument slice. Like SyscallN, a uintptr converted from an
// unsafe.Pointer in the call expression keeps its object alive and un-moved
// for the duration of the call (go:uintptrescapes); the object escapes to
// the heap. The C function must not retain the pointer after returning.
// Mixed integer/float arguments are not supported, as for SyscallN.
//
//go:uintptrescapes
func Syscall15(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15 uintptr) (r1, r2, err uintptr) {
	if fn == 0 {
		panic("purego: fn is nil")
	}
	if runtime.GOOS == "windows" {
		return syscall_syscallN(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15)
	}
	s := syscallArgs{
		fn: fn, a1: a1, a2: a2, a3: a3, a4: a4, a5: a5,
		a6: a6, a7: a7, a8: a8, a9: a9, a10: a10,
		a11: a11, a12: a12, a13: a13, a14: a14, a15: a15,
	}
	// cgocall is synchronous: s remains on this goroutine's stack until it returns.
	runtime_cgocall(syscallXABI0, noescapeFixed(unsafe.Pointer(&s)))
	return s.a1, s.a2, s.a3
}

// Syscall6 is Syscall15 with six arguments. Like Syscall15, pointer-derived
// uintptr arguments keep their objects alive and cause them to escape.
//
//go:uintptrescapes
func Syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr) {
	return Syscall15(fn, a1, a2, a3, a4, a5, a6, 0, 0, 0, 0, 0, 0, 0, 0, 0)
}
