// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build darwin || freebsd || linux || netbsd

package purego_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/bnema/purego"
	"github.com/bnema/purego/internal/load"
)

func TestRegisterFuncStringSliceAndArray(t *testing.T) {
	libFileName := filepath.Join(t.TempDir(), "slicetest.so")
	if err := buildSharedLib(t, "CC", libFileName, filepath.Join("testdata", "slicetest", "slice_test.c")); err != nil {
		t.Fatal(err)
	}
	lib, err := load.OpenLibrary(libFileName)
	if err != nil {
		t.Fatalf("Dlopen(%q) failed: %v", libFileName, err)
	}
	defer load.CloseLibrary(lib)

	var join func([]string) string
	purego.RegisterLibFunc(&join, lib, "join_strings")
	var get func(int32) []string
	purego.RegisterLibFunc(&get, lib, "get_strings")
	var sum func([4]float32) float32
	purego.RegisterLibFunc(&sum, lib, "sum_floats4")

	t.Run("[]string argument", func(t *testing.T) {
		if got := join([]string{"a", "bc", "def"}); got != "a,bc,def" {
			t.Errorf("join = %q, want %q", got, "a,bc,def")
		}
		if got := join([]string{}); got != "" {
			t.Errorf("join(empty) = %q, want empty", got)
		}
		if got := join(nil); got != "(null)" {
			t.Errorf("join(nil) = %q, want %q", got, "(null)")
		}
	})

	t.Run("[]string return", func(t *testing.T) {
		if got, want := get(0), []string{"alpha", "beta", "gamma"}; !slices.Equal(got, want) {
			t.Errorf("get = %q, want %q", got, want)
		}
		if got := get(1); got != nil {
			t.Errorf("get(NULL) = %q, want nil", got)
		}
	})

	t.Run("array argument", func(t *testing.T) {
		if got := sum([4]float32{1, 2, 3, 4.5}); got != 10.5 {
			t.Errorf("sum = %v, want 10.5", got)
		}
	})
}
