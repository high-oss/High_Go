// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	highopenapi "github.com/high-oss/High_Go"
)

// Every operation the SDK covers, as a method name per resource — the same
// table contract §1 and the READMEs of every language SDK carry.
var expectedSurface = map[string][]string{
	"Auth":      {"GenerateAccessToken"},
	"Orders":    {"Place", "Modify", "Get", "Cancel", "List", "Trades", "TradesFor", "Charges", "Margin"},
	"Portfolio": {"Positions", "Holdings", "Funds", "ConvertPosition", "ExitAllPositions", "ExitPosition"},
	"Scrips":    {"Quotes", "Ohlc", "Depth", "Expiries", "FutureData", "Historical", "OptionChain"},
	"Market":    {"Status"},
}

func TestPublicSurfaceExposesExactlyTheContractsMethodSet(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{}))
	})
	defer ts.Close()

	client := clientFor(t, ts, nil)
	resources := map[string]any{
		"Auth": client.Auth, "Orders": client.Orders, "Portfolio": client.Portfolio,
		"Scrips": client.Scrips, "Market": client.Market,
	}

	total := 0
	for name, want := range expectedSurface {
		resource, ok := resources[name]
		if !ok {
			t.Fatalf("client.%s is missing", name)
		}
		got := methodNames(resource)
		if !sameSet(got, want) {
			t.Fatalf("%s methods = %v, want %v", name, got, want)
		}
		total += len(want)
	}

	// The spec has 27 operations; the SDK covers 24 — login, the two consent
	// operations and token introspection are deliberately excluded.
	if total != 24 {
		t.Fatalf("total methods = %d, want 24", total)
	}
}

func methodNames(v any) []string {
	t := reflect.TypeOf(v)
	out := make([]string, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		out[i] = t.Method(i).Name
	}
	return out
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// The excluded auth operations — the browser login page, the redirect
// consent flow, and token introspection — must not exist under any spelling.
func TestExcludedAuthOperationsDoNotExist(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{}))
	})
	defer ts.Close()

	authType := reflect.TypeOf(clientFor(t, ts, nil).Auth)
	excluded := []string{
		"Login", "LoginURL", "RenderLogin",
		"GenerateConsent", "ConsumeConsent", "ValidateToken",
	}
	for _, name := range excluded {
		if _, ok := authType.MethodByName(name); ok {
			t.Errorf("Auth.%s must not exist", name)
		}
	}
}

// The only two dependencies this module declares are the oapi-codegen
// runtime support package the generated types need (UUID/Date helpers, the
// Error.message oneOf union) and coder/websocket, the one new runtime
// dependency the datafeed socket is permitted (contract §9) — no
// hand-written business logic depends on anything else outside the standard
// library.
func TestGoModDeclaresOnlyTheCodegenRuntimeDependency(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	var direct []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
		case inBlock && trimmed == ")":
			inBlock = false
		case strings.HasPrefix(trimmed, "require ") && !strings.Contains(trimmed, "("):
			direct = append(direct, fieldsFirst(strings.TrimPrefix(trimmed, "require ")))
		case inBlock && trimmed != "" && !strings.Contains(trimmed, "// indirect"):
			direct = append(direct, fieldsFirst(trimmed))
		}
	}
	want := []string{"github.com/coder/websocket", "github.com/oapi-codegen/runtime"}
	sort.Strings(direct)
	if !sameSet(direct, want) || len(direct) != len(want) {
		t.Fatalf("go.mod direct requires = %v, want exactly %v", direct, want)
	}
}

func fieldsFirst(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
