// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

//go:build darwin || freebsd || linux || netbsd || windows

package purego

// SyscallSelf calls fn with self as the first argument followed by args.
// It is equivalent to SyscallN(fn, append([]uintptr{self}, args...)...)
// without allocating the combined argument slice.
//
//go:uintptrescapes
func SyscallSelf(fn, self uintptr, args ...uintptr) (r1, r2, err uintptr) {
	if fn == 0 {
		panic("purego: fn is nil")
	}
	if len(args) > maxArgs-1 {
		panic("purego: too many arguments to SyscallSelf")
	}
	var tmp [maxArgs]uintptr
	tmp[0] = self
	n := copy(tmp[1:], args)
	return syscallPadded(fn, &tmp, n+1)
}
