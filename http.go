// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// authKind names which credential an operation needs, from the spec's
// `security` block: x-api-key on generateAccessToken, bearer on the other 23.
type authKind int

const (
	authBearer authKind = iota
	authAPIKey
	// authNone attaches neither credential. Used for the instrument list
	// manifest, which the spec marks `security: []` — it and the CSV files it
	// points at are public, and the CSV host is a third-party CDN that must
	// never see the caller's bearer token or API key.
	authNone
)

// requestOptions describes one operation call. path is the operation path
// template, already interpolated via pathOf. query only carries parameters
// that are actually set — the resource layer omits anything undefined rather
// than passing an empty value through.
type requestOptions struct {
	method string
	path   string
	auth   authKind
	query  map[string]string
	body   any
}

// retryableStatuses are worth trying again on an idempotent operation.
var retryableStatuses = map[int]bool{429: true, 500: true, 502: true, 503: true, 504: true}

func isIdempotent(method string) bool {
	m := strings.ToUpper(method)
	return m == http.MethodGet || m == http.MethodHead
}

// envelope is the `{requestId, data}` shape every successful response
// carries. Data is decoded lazily into the caller's type so this struct never
// needs to know the operation's response shape.
type envelope struct {
	RequestID string          `json:"requestId"`
	Data      json.RawMessage `json:"data"`
}

func headersFor(config *resolvedConfig, opts requestOptions) (http.Header, *APIError) {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", config.userAgent)

	switch opts.auth {
	case authBearer:
		if config.accessToken == "" {
			return nil, newPreflightError("This operation needs an accessToken. Pass it to the client, or set HIGH_ACCESS_TOKEN.")
		}
		headers.Set("Authorization", "Bearer "+config.accessToken)
	case authAPIKey:
		if config.apiKey == "" {
			return nil, newPreflightError("This operation needs an apiKey. Pass it to the client, or set HIGH_API_KEY.")
		}
		headers.Set("x-api-key", config.apiKey)
	}

	if opts.body != nil {
		headers.Set("Content-Type", "application/json")
	}
	return headers, nil
}

func urlFor(config *resolvedConfig, opts requestOptions) string {
	full := buildURL(config, opts.path)
	if len(opts.query) == 0 {
		return full
	}
	parsed, err := url.Parse(full)
	if err != nil {
		return full
	}
	q := parsed.Query()
	for k, v := range opts.query {
		q.Set(k, v)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// retryAfterMs parses a Retry-After header. The spec allows either a delay in
// seconds or an HTTP-date; treating it as a number unconditionally yields NaN
// for the date form, so both are handled and anything else is ignored.
func retryAfterMs(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(header, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds * float64(time.Second)), true
	}
	if at, err := http.ParseTime(header); err == nil {
		d := at.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// sleepCtx waits d, or returns ctx's cancellation cause as soon as it is
// cancelled. An unabortable sleep would let a cancelled call go on to issue
// its next retry — precisely what a caller asked not to happen.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

type attemptResult struct {
	status      int
	raw         []byte
	apiErr      *APIError
	retryAfter  time.Duration
	hasRetryHdr bool
}

// doAttempt performs one HTTP exchange. It never returns an error for an API
// error status — the caller's retry policy decides — but does return one for
// a transport failure it cannot classify as an API response.
func doAttempt(ctx context.Context, config *resolvedConfig, opts requestOptions, fullURL string, headers http.Header) (attemptResult, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(config.timeoutMs)*time.Millisecond)
	defer cancel()

	var payload []byte
	var bodyReader io.Reader
	if opts.body != nil {
		encoded, err := json.Marshal(opts.body)
		if err != nil {
			return attemptResult{}, newTransportError("failed to encode the request body", err)
		}
		payload = encoded
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(attemptCtx, opts.method, fullURL, bodyReader)
	if err != nil {
		return attemptResult{}, newTransportError("failed to build the request", err)
	}
	req.Header = headers

	// Headers are never logged — they carry the bearer token and the api key.
	// The body is, with credential-shaped keys masked and oversized payloads
	// replaced by a note, so debug shows what was called and what was sent.
	if config.logger.enabled(LogLevelDebug) {
		var detail any
		if payload != nil {
			detail = map[string]any{"body": redactBody(payload)}
		}
		config.logger.Debug(fmt.Sprintf("HIGH -> %s %s", strings.ToUpper(opts.method), redactURL(fullURL)), detail)
	}
	config.logger.Info(fmt.Sprintf("HIGH %s %s", strings.ToUpper(opts.method), redactURL(fullURL)), nil)

	startedAt := time.Now()
	resp, err := config.httpClient.Do(req)
	if err != nil {
		return attemptResult{}, classifyTransportError(ctx, attemptCtx, config, fullURL, err, "")
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return attemptResult{}, classifyTransportError(ctx, attemptCtx, config, fullURL, err, " while reading the response body")
	}

	elapsed := time.Since(startedAt)

	var reqID string
	var env envelope
	if len(raw) > 0 && json.Unmarshal(raw, &env) == nil {
		reqID = env.RequestID
	}
	if config.logger.enabled(LogLevelDebug) {
		config.logger.Debug(fmt.Sprintf("HIGH <- %d in %dms %s", resp.StatusCode, elapsed.Milliseconds(), redactURL(fullURL)),
			map[string]any{"requestId": reqID, "body": redactBody(raw)})
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := errorFromResponse(resp.StatusCode, raw)
		config.logger.Error(
			fmt.Sprintf("HIGH <- %d %s in %dms %s", resp.StatusCode, orNoCode(apiErr.Code), elapsed.Milliseconds(), redactURL(fullURL)),
			map[string]any{"requestId": apiErr.RequestID, "messages": apiErr.Messages},
		)
		delay, hasHeader := retryAfterMs(resp.Header.Get("Retry-After"), time.Now())
		return attemptResult{status: resp.StatusCode, raw: raw, apiErr: apiErr, retryAfter: delay, hasRetryHdr: hasHeader}, nil
	}

	return attemptResult{status: resp.StatusCode, raw: raw}, nil
}

func orNoCode(code string) string {
	if code == "" {
		return "no code"
	}
	return code
}

// classifyTransportError distinguishes a caller cancellation (surfaced
// unchanged, never a timeout, never retried) from the SDK's own timeout
// (status 0, a clear "timed out" message) from anything else.
func classifyTransportError(callerCtx, attemptCtx context.Context, config *resolvedConfig, fullURL string, cause error, phase string) error {
	if callerCtx.Err() != nil {
		return context.Cause(callerCtx)
	}
	if attemptCtx.Err() != nil {
		config.logger.Error(fmt.Sprintf("HIGH <- timeout after %dms%s %s", config.timeoutMs, phase, redactURL(fullURL)), nil)
		return newTransportError(fmt.Sprintf("Request timed out after %dms%s", config.timeoutMs, phase), nil)
	}
	config.logger.Error(fmt.Sprintf("HIGH <- transport failure %s", redactURL(fullURL)), cause.Error())
	return newTransportError("Request failed", cause)
}

// doRequest sends one operation and returns its unwrapped `data`. Retries
// apply to idempotent reads only (GET/HEAD, case-insensitive), on 429 and
// 5xx, honouring Retry-After when present and backing off exponentially with
// jitter when it is not. Writes are never retried.
func doRequest[T any](ctx context.Context, config *resolvedConfig, opts requestOptions) (T, error) {
	var zero T

	headers, preflightErr := headersFor(config, opts)
	if preflightErr != nil {
		return zero, preflightErr
	}

	fullURL := urlFor(config, opts)
	retryable := isIdempotent(opts.method)
	maxAttempts := 1
	if retryable {
		maxAttempts = config.maxRetries + 1
	}

	var last attemptResult
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := doAttempt(ctx, config, opts, fullURL, headers)
		if err != nil {
			return zero, err
		}
		last = result

		if last.apiErr == nil {
			break
		}
		if !retryable || !retryableStatuses[last.status] || attempt == maxAttempts-1 {
			return zero, last.apiErr
		}

		backoff := time.Duration(250*(1<<attempt))*time.Millisecond + time.Duration(rand.Intn(100))*time.Millisecond
		delay := backoff
		if last.hasRetryHdr {
			delay = last.retryAfter
		}

		// A Retry-After past the ceiling means "come back much later", not
		// "block this call". A misconfigured rate limiter or an edge WAF can
		// legally send Retry-After: 86400, and waiting it out inside a call
		// the caller cannot interrupt is worse than failing now.
		maxDelay := time.Duration(config.maxRetryDelayMs) * time.Millisecond
		if delay > maxDelay {
			config.logger.Warn(fmt.Sprintf(
				"HIGH giving up: server asked for %dms, over the %dms ceiling %s",
				delay.Milliseconds(), config.maxRetryDelayMs, redactURL(fullURL)), nil)
			return zero, last.apiErr
		}

		config.logger.Warn(fmt.Sprintf("HIGH retry %d/%d after %dms (HTTP %d) %s",
			attempt+1, maxAttempts-1, delay.Milliseconds(), last.status, redactURL(fullURL)), nil)

		if err := sleepCtx(ctx, delay); err != nil {
			return zero, err
		}
	}

	if last.apiErr != nil {
		return zero, last.apiErr
	}

	if len(last.raw) == 0 {
		return zero, nil
	}
	var env envelope
	if err := json.Unmarshal(last.raw, &env); err != nil {
		return zero, &APIError{Status: last.status, msg: "Expected JSON but the response was not parseable", Body: string(last.raw)}
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return zero, nil
	}
	var result T
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return zero, &APIError{Status: last.status, msg: fmt.Sprintf("Failed to decode the response: %s", err.Error()), Body: string(last.raw)}
	}
	return result, nil
}
