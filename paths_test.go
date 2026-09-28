// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"strings"
	"testing"
)

func TestPathOfInterpolatesAPlainParameter(t *testing.T) {
	got, err := pathOf("/orders/{orderId}", map[string]string{"orderId": "2609250000123456"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/orders/2609250000123456" {
		t.Errorf("pathOf = %q", got)
	}
}

// Review Focus 5 (Node).
func TestPathOfEncodesCharactersThatWouldChangeThePath(t *testing.T) {
	got, err := pathOf("/scrips/{symbol}/depth", map[string]string{"symbol": "M&M-EQ"})
	if err != nil || got != "/scrips/M%26M-EQ/depth" {
		t.Errorf("pathOf = %q, err = %v", got, err)
	}
	got, err = pathOf("/scrips/{symbol}/depth", map[string]string{"symbol": "NIFTY 50"})
	if err != nil || got != "/scrips/NIFTY%2050/depth" {
		t.Errorf("pathOf = %q, err = %v", got, err)
	}
}

func TestPathOfEncodesSlashSoAParamCannotAddASegment(t *testing.T) {
	got, err := pathOf("/scrips/{symbol}/depth", map[string]string{"symbol": "a/../b"})
	if err != nil || got != "/scrips/a%2F..%2Fb/depth" {
		t.Errorf("pathOf = %q, err = %v", got, err)
	}
}

func TestPathOfInterpolatesSeveralParameters(t *testing.T) {
	got, err := pathOf("/scrips/{symbol}/{type}/expiries", map[string]string{"symbol": "NIFTY", "type": "OPT"})
	if err != nil || got != "/scrips/NIFTY/OPT/expiries" {
		t.Errorf("pathOf = %q, err = %v", got, err)
	}
}

func TestPathOfRefusesAMissingParameter(t *testing.T) {
	_, err := pathOf("/orders/{orderId}", map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "orderId") {
		t.Fatalf("expected an error naming orderId, got %v", err)
	}
}
