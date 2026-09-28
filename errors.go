// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ErrorCodes is the documented error catalogue, from the spec's
// `Error.code['x-error-codes']`.
//
// It is a slice and not a closed Go type on purpose: the gateway defines far
// more codes than the catalogue documents and passes most through unchanged,
// so a closed enum would make APIError.Code unassignable for a code that is
// perfectly real. Compare against these constants; expect others.
var ErrorCodes = []string{
	"INVALID_API_KEY",
	"INVALID_CONSENT",
	"INVALID_DATE",
	"INVALID_SCRIP",
	"INVALID_TOKEN",
	"INVALID_TOKEN_ID",
	"INVALID_TOTP",
	"ORDER_DETAILS_INVALID",
	"ORDER_NOT_CANCELLABLE",
	"ORDER_NOT_FOUND",
	"ORDER_REJECTED",
	"ORDER_STATUS_UNKNOWN",
	"OWNER_MISMATCH",
	"POSITION_CONVERT_FAILED",
	"POSITION_NOT_EXITABLE",
	"POSITION_SQUARE_OFF_FAILED",
	"QUANTITY_NOT_IN_LOTS",
	"REDIRECT_URL_NOT_CONFIGURED",
	"SANDBOX_TOKEN_NOT_ALLOWED",
	"SCRIP_NOT_FOUND",
	"SERVICE_UNAVAILABLE",
	"SESSION_EXPIRED",
	"SESSION_NOT_GENERATED",
	"STATIC_IP_MISMATCH",
	"STATIC_IP_MISSING",
	"TOTP_NOT_ENABLED",
	"UNHANDLED_ERROR",
	"VALIDATION_ERROR",
	"WRONG_TOKEN_AUDIENCE",
}

// APIError is every failure the SDK surfaces — transport, timeout, or an API
// error response. It is the one exported error type; every non-nil error
// returned by a Client method is an *APIError, recoverable with errors.As.
type APIError struct {
	// Status is the HTTP status, or 0 for a transport failure or timeout.
	Status int
	// Code is an open string: the documented catalogue is ErrorCodes, but the
	// gateway emits others. Empty when the failure never reached the gateway.
	Code string
	// RequestID: quote this in support tickets. Empty when there was none.
	RequestID string
	// Messages holds every message the API returned; validation errors
	// return several. Nil when the body carried none.
	Messages []string
	// Body is the parsed payload when there was one — a map[string]any for a
	// JSON error body, or a string for a non-JSON one (an HTML 502 from a
	// proxy). Nil otherwise.
	Body any

	msg string
}

// Error implements the error interface.
func (e *APIError) Error() string { return e.msg }

func newTransportError(message string, cause error) *APIError {
	msg := message
	if cause != nil {
		msg = fmt.Sprintf("%s: %s", message, cause.Error())
	}
	return &APIError{Status: 0, msg: msg, Body: cause}
}

func newPreflightError(message string) *APIError {
	return &APIError{Status: 0, msg: message}
}

// errorFromResponse builds the error for a non-2xx response. raw is the
// response body exactly as received; a non-JSON body (an HTML 502 from a
// load balancer or proxy) still surfaces with the real status rather than a
// JSON decode failure.
func errorFromResponse(status int, raw []byte) *APIError {
	var record map[string]any
	parsed := len(raw) > 0 && json.Unmarshal(raw, &record) == nil

	var code, requestID string
	var messages []string
	var body any

	if parsed {
		code, _ = record["code"].(string)
		requestID, _ = record["requestId"].(string)
		messages = messagesOf(record["message"])
		body = record
	} else if len(raw) > 0 {
		body = string(raw)
	}

	snippet := strings.TrimSpace(string(raw))
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}

	var msg string
	switch {
	case len(messages) > 0:
		msg = strings.Join(messages, "; ")
	case snippet != "":
		msg = fmt.Sprintf("HTTP %d: %s", status, snippet)
	default:
		msg = fmt.Sprintf("HTTP %d", status)
	}

	return &APIError{Status: status, Code: code, RequestID: requestID, Messages: messages, Body: body, msg: msg}
}

// messagesOf handles the Error schema's `message`, which is `oneOf [string,
// array of string]` — validation errors return several.
func messagesOf(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}
