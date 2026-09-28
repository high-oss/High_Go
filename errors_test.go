// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"errors"
	"strings"
	"testing"
)

func TestAPIErrorIsAnErrorWithUsefulFields(t *testing.T) {
	err := errorFromResponse(400, []byte(`{"code":"ORDER_REJECTED","message":"Order rejected"}`))
	var target *APIError
	if !errors.As(error(err), &target) {
		t.Fatal("expected errors.As to match *APIError")
	}
	if err.Error() != "Order rejected" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Status != 400 {
		t.Errorf("Status = %d", err.Status)
	}
	if err.Code != "ORDER_REJECTED" {
		t.Errorf("Code = %q", err.Code)
	}
}

func TestErrorFromResponseMapsTheDocumentedEnvelope(t *testing.T) {
	err := errorFromResponse(400, []byte(`{"requestId":"a1b2c3","code":"ORDER_REJECTED","message":"Insufficient funds"}`))
	if err.Status != 400 || err.Code != "ORDER_REJECTED" || err.RequestID != "a1b2c3" {
		t.Fatalf("unexpected error: %+v", err)
	}
	if err.Error() != "Insufficient funds" {
		t.Errorf("Error() = %q", err.Error())
	}
	if len(err.Messages) != 1 || err.Messages[0] != "Insufficient funds" {
		t.Errorf("Messages = %v", err.Messages)
	}
}

// The Error schema's message is oneOf [string, array] — validation errors
// return a list, and every one of them must survive.
func TestErrorFromResponseKeepsEveryMessageInAList(t *testing.T) {
	err := errorFromResponse(400, []byte(`{"requestId":"a1b2c3","code":"VALIDATION_ERROR","message":["quantity must be positive","price is required"]}`))
	want := []string{"quantity must be positive", "price is required"}
	if len(err.Messages) != 2 || err.Messages[0] != want[0] || err.Messages[1] != want[1] {
		t.Fatalf("Messages = %v", err.Messages)
	}
	if err.Error() != "quantity must be positive; price is required" {
		t.Errorf("Error() = %q", err.Error())
	}
}

// Review Focus 3 (Node): an ALB or proxy returns HTML, not JSON.
func TestErrorFromResponseSurvivesNonJSONBody(t *testing.T) {
	err := errorFromResponse(502, []byte("<html><body>502 Bad Gateway</body></html>"))
	if err.Status != 502 {
		t.Errorf("Status = %d", err.Status)
	}
	if err.Code != "" {
		t.Errorf("Code = %q, want empty", err.Code)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("Error() = %q, want to mention 502", err.Error())
	}
}

func TestErrorFromResponseFallsBackToStatusWithNoMessage(t *testing.T) {
	err := errorFromResponse(500, []byte(`{"requestId":"a1b2c3"}`))
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Error() = %q, want to mention 500", err.Error())
	}
	if err.RequestID != "a1b2c3" {
		t.Errorf("RequestID = %q", err.RequestID)
	}
}

func TestErrorFromResponseAcceptsCodeOutsideCatalogue(t *testing.T) {
	err := errorFromResponse(403, []byte(`{"code":"DATA_PLAN_REQUIRED","message":"Upgrade needed"}`))
	if err.Code != "DATA_PLAN_REQUIRED" {
		t.Errorf("Code = %q", err.Code)
	}
}

func TestErrorFromResponseEmptyBody(t *testing.T) {
	err := errorFromResponse(500, nil)
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestErrorCodesCarriesTheDocumentedCatalogue(t *testing.T) {
	has := func(code string) bool {
		for _, c := range ErrorCodes {
			if c == code {
				return true
			}
		}
		return false
	}
	if !has("ORDER_REJECTED") || !has("VALIDATION_ERROR") {
		t.Fatal("ErrorCodes missing a documented code")
	}
	if len(ErrorCodes) <= 20 {
		t.Fatalf("ErrorCodes too short: %d", len(ErrorCodes))
	}
}
