// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

//go:build darwin || freebsd || linux || netbsd || windows

package purego_test

import (
	"fmt"
	"testing"

	"github.com/bnema/purego"
)

func TestSyscallSelf(t *testing.T) {
	sum2 := purego.NewCallback(func(self, a1 uintptr) uintptr { return self + a1 })
	sum4 := purego.NewCallback(func(self, a1, a2, a3 uintptr) uintptr { return self + a1 + a2 + a3 })
	triple := purego.NewCallback(func(self uintptr) uintptr { return self * 3 })

	tests := []struct {
		name string
		fn   uintptr
		self uintptr
		args []uintptr
		want uintptr
	}{
		{"one arg", sum2, 10, []uintptr{20}, 30},
		{"zero self", sum2, 0, []uintptr{42}, 42},
		{"multiple args", sum4, 1, []uintptr{2, 3, 4}, 10},
		{"no variadic args", triple, 7, nil, 21},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if r1, _, _ := purego.SyscallSelf(tc.fn, tc.self, tc.args...); r1 != tc.want {
				t.Errorf("SyscallSelf returned %d, want %d", r1, tc.want)
			}
		})
	}
}

func TestSyscallSelfPanics(t *testing.T) {
	t.Run("nil fn", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic for nil fn")
			}
		}()
		purego.SyscallSelf(0, 1)
	})

	t.Run("too many args", func(t *testing.T) {
		defer func() {
			if got := fmt.Sprint(recover()); got != "purego: too many arguments to SyscallSelf" {
				t.Errorf("unexpected panic: %s", got)
			}
		}()
		fn := purego.NewCallback(func(uintptr) uintptr { return 0 })
		purego.SyscallSelf(fn, 1, make([]uintptr, purego.MaxArgs)...)
	})
}

func TestSyscallSelfAllocs(t *testing.T) {
	fn := purego.NewCallback(func(self, a1, a2 uintptr) uintptr { return self + a1 + a2 })
	directAllocs := testing.AllocsPerRun(100, func() { purego.SyscallN(fn, 1, 2, 3) })
	selfAllocs := testing.AllocsPerRun(100, func() { purego.SyscallSelf(fn, 1, 2, 3) })
	if selfAllocs > directAllocs {
		t.Fatalf("SyscallSelf allocs = %v, SyscallN allocs = %v", selfAllocs, directAllocs)
	}
}
