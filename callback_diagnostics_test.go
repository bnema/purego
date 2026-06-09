// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

//go:build darwin || freebsd || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || (cgo && s390x))) || netbsd

package purego

import "testing"

func TestCallbackDiagnosticsDisabledByDefault(t *testing.T) {
	t.Setenv("PUREGO_CALLBACK_LEDGER", "")
	t.Setenv("PUREGO_CALLBACK_LEDGER_STACK", "")
	t.Setenv("PUREGO_CALLBACK_TRACE", "")

	if callbackLedgerEnabled() {
		t.Fatal("callback ledger must be disabled by default")
	}
	if callbackLedgerStackEnabled() {
		t.Fatal("callback ledger stack capture must be disabled by default")
	}
	if callbackTraceEnabled() {
		t.Fatal("callback trace must be disabled by default")
	}
}

func TestCallbackDiagnosticsExplicitOptIn(t *testing.T) {
	for _, value := range []string{"1", "true", "yes", "on", "TRUE", "On"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PUREGO_CALLBACK_LEDGER", value)
			if !callbackLedgerEnabled() {
				t.Fatalf("callback ledger was not enabled by %q", value)
			}
		})
	}
}

func TestCallbackDiagnosticsRejectsImplicitValues(t *testing.T) {
	for _, value := range []string{"0", "false", "no", "off", "anything-else"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PUREGO_CALLBACK_LEDGER", value)
			if callbackLedgerEnabled() {
				t.Fatalf("callback ledger unexpectedly enabled by %q", value)
			}
		})
	}
}
