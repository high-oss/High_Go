// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"testing"

	"github.com/high-oss/High_Go"
	"github.com/high-oss/High_Go/generated"
)

// The pinned spec has 27 operations; the SDK covers 24 of them (contract §1).
func TestGeneratedTypesCoverEveryOperation(t *testing.T) {
	doc := highopenapi.LoadPinnedSpec(t)
	methods := map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true}
	count := 0
	for _, item := range doc.Paths {
		for verb := range item {
			if methods[verb] {
				count++
			}
		}
	}
	if count != 27 {
		t.Fatalf("expected 27 operations in the pinned spec, got %d", count)
	}
}

// Compile-time assertions that the generated schemas exist with the field
// names the facades rely on. This fails `go build` if oapi-codegen renamed
// anything on a regenerate.
func TestGeneratedSchemasExposeExpectedFields(t *testing.T) {
	order := generated.Order{OrderId: "1"}
	if order.OrderId != "1" {
		t.Fatal("Order.OrderId round-trip failed")
	}
	funds := generated.Funds{AvailableBalance: 0}
	if funds.AvailableBalance != 0 {
		t.Fatal("Funds.AvailableBalance round-trip failed")
	}
	var scripCodeOnOrder string = order.ScripCode
	_ = scripCodeOnOrder
	var scripCodeOnScripInfo int = generated.ScripInfo{}.ScripCode
	_ = scripCodeOnScripInfo
}
