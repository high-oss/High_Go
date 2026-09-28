// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	highopenapi "github.com/high-live/high-openapi"
)

func clientFor(t *testing.T, ts *highopenapi.TestServer, apply func(*highopenapi.Options)) *highopenapi.Client {
	t.Helper()
	opts := highopenapi.Options{BaseURL: ts.URL, APIKey: "key", AccessToken: "tok"}
	if apply != nil {
		apply(&opts)
	}
	client, err := highopenapi.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAuthGenerateAccessTokenSendsQueryParamsWithAPIKey(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"accessToken": "jwt", "expiresAt": "2026-09-28T06:30:56.123Z",
		}))
	})
	defer ts.Close()

	token, err := clientFor(t, ts, nil).Auth.GenerateAccessToken(context.Background(), "C1", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "jwt" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	req := ts.Req(0)
	if !strings.Contains(req.URL, "clientId=C1") || !strings.Contains(req.URL, "tOtp=123456") {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Header.Get("x-api-key") != "key" {
		t.Errorf("x-api-key = %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("Authorization should be empty, got %q", req.Header.Get("Authorization"))
	}
}

// The excluded auth operations must not exist as methods, under any spelling.
func TestAuthExposesOnlyGenerateAccessToken(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{}))
	})
	defer ts.Close()

	authType := reflect.TypeOf(clientFor(t, ts, nil).Auth)
	methods := map[string]bool{}
	for i := 0; i < authType.NumMethod(); i++ {
		methods[authType.Method(i).Name] = true
	}
	if len(methods) != 1 || !methods["GenerateAccessToken"] {
		t.Fatalf("AuthResource method set = %v, want exactly {GenerateAccessToken}", methods)
	}
}
