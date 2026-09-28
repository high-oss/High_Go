// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

// SpecDoc is the minimal slice of the pinned OpenAPI document the test suite
// cross-checks generated/hand-written code against. Exported so both the
// internal (package highopenapi) and external (package highopenapi_test)
// test files share one loader and one real copy of the spec.
type SpecDoc struct {
	Servers []struct {
		URL         string `json:"url"`
		Environment string `json:"x-environment"`
	} `json:"servers"`
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

// LoadPinnedSpec reads generated/openapi.json — the spec at the commit
// spec.lock.json pins, copied in by the (manual, in this repo) regenerate
// step. Never the working tree of ../spec.
func LoadPinnedSpec(t *testing.T) SpecDoc {
	t.Helper()
	data, err := os.ReadFile("generated/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc SpecDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func originOf(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}
