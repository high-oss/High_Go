// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi

import (
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func mustResolve(t *testing.T, opts Options) *resolvedConfig {
	t.Helper()
	if opts.getenv == nil {
		opts.getenv = noopGetenv
	}
	config, err := resolveConfig(opts)
	if err != nil {
		t.Fatalf("resolveConfig: %v", err)
	}
	return config
}

func TestResolveConfigDefaults(t *testing.T) {
	config := mustResolve(t, Options{})
	if config.baseURL != "https://openapi.high.live" {
		t.Errorf("baseURL = %q", config.baseURL)
	}
	if config.versionPath != "v1" {
		t.Errorf("versionPath = %q", config.versionPath)
	}
	if config.timeoutMs != 30_000 {
		t.Errorf("timeoutMs = %d", config.timeoutMs)
	}
	if config.maxRetries != 2 {
		t.Errorf("maxRetries = %d", config.maxRetries)
	}
}

func TestResolveConfigSandboxHost(t *testing.T) {
	config := mustResolve(t, Options{Environment: EnvironmentSandbox})
	if config.baseURL != Environments[EnvironmentSandbox].API {
		t.Errorf("baseURL = %q", config.baseURL)
	}
}

// Reserved for the datafeed socket (contract §9): resolving it here means the
// feed reuses this configuration instead of introducing a second one.
func TestResolveConfigWebsocketHost(t *testing.T) {
	if mustResolve(t, Options{}).wsBaseURL != Environments[EnvironmentProduction].WS {
		t.Fatal("production wsBaseURL mismatch")
	}
	config := mustResolve(t, Options{Environment: EnvironmentSandbox})
	if config.wsBaseURL != Environments[EnvironmentSandbox].WS {
		t.Fatal("sandbox wsBaseURL mismatch")
	}
}

func TestResolveConfigWSBaseURLOverrideIndependent(t *testing.T) {
	config := mustResolve(t, Options{WSBaseURL: "ws://127.0.0.1:9001"})
	if config.wsBaseURL != "ws://127.0.0.1:9001" {
		t.Errorf("wsBaseURL = %q", config.wsBaseURL)
	}
	if config.baseURL != Environments[EnvironmentProduction].API {
		t.Errorf("baseURL should be untouched, got %q", config.baseURL)
	}
}

func TestResolveConfigExplicitBaseURLBeatsEnvironment(t *testing.T) {
	config := mustResolve(t, Options{Environment: EnvironmentSandbox, BaseURL: "http://localhost:8080"})
	if config.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q", config.baseURL)
	}
}

func TestResolveConfigReadsEnvVarsWhenNotPassed(t *testing.T) {
	config := mustResolve(t, Options{getenv: envFrom(map[string]string{
		"HIGH_ENVIRONMENT": "sandbox", "HIGH_API_KEY": "k", "HIGH_ACCESS_TOKEN": "t",
	})})
	if config.baseURL != Environments[EnvironmentSandbox].API {
		t.Errorf("baseURL = %q", config.baseURL)
	}
	if config.apiKey != "k" || config.accessToken != "t" {
		t.Errorf("credentials = %q / %q", config.apiKey, config.accessToken)
	}
}

func TestResolveConfigExplicitBeatsEnvVars(t *testing.T) {
	config := mustResolve(t, Options{APIKey: "explicit", getenv: envFrom(map[string]string{"HIGH_API_KEY": "from-env"})})
	if config.apiKey != "explicit" {
		t.Errorf("apiKey = %q", config.apiKey)
	}
}

func TestResolveConfigRejectsUnknownEnvironment(t *testing.T) {
	_, err := resolveConfig(Options{Environment: "staging", getenv: noopGetenv})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"staging", "production", "sandbox"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestResolveConfigDefaultsLogLevelSilent(t *testing.T) {
	if mustResolve(t, Options{}).logLevel != LogLevelSilent {
		t.Fatal("expected silent by default")
	}
}

func TestResolveConfigLogLevelFromOptionsOrEnv(t *testing.T) {
	if mustResolve(t, Options{LogLevel: LogLevelDebug}).logLevel != LogLevelDebug {
		t.Error("options logLevel not honoured")
	}
	if mustResolve(t, Options{getenv: envFrom(map[string]string{"HIGH_LOG_LEVEL": "warn"})}).logLevel != LogLevelWarn {
		t.Error("HIGH_LOG_LEVEL not honoured")
	}
	config := mustResolve(t, Options{LogLevel: LogLevelInfo, getenv: envFrom(map[string]string{"HIGH_LOG_LEVEL": "warn"})})
	if config.logLevel != LogLevelInfo {
		t.Error("options should win over env var")
	}
}

func TestResolveConfigRejectsUnknownLogLevel(t *testing.T) {
	if _, err := resolveConfig(Options{LogLevel: "verbose", getenv: noopGetenv}); err == nil || !strings.Contains(err.Error(), "verbose") {
		t.Fatalf("expected an error naming 'verbose', got %v", err)
	}
	if _, err := resolveConfig(Options{getenv: envFrom(map[string]string{"HIGH_LOG_LEVEL": "trace"})}); err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("expected an error naming 'trace', got %v", err)
	}
}

func TestResolveConfigRejectsUnknownEnvVarEnvironment(t *testing.T) {
	_, err := resolveConfig(Options{getenv: envFrom(map[string]string{"HIGH_ENVIRONMENT": "prod"})})
	if err == nil || !strings.Contains(err.Error(), "prod") {
		t.Fatalf("expected an error naming 'prod', got %v", err)
	}
}

func TestResolveConfigRejectsNonPositiveTimeout(t *testing.T) {
	if _, err := resolveConfig(Options{TimeoutMs: Ptr(0), getenv: noopGetenv}); err == nil || !strings.Contains(err.Error(), "timeoutMs") {
		t.Fatalf("expected a timeoutMs error, got %v", err)
	}
	if _, err := resolveConfig(Options{TimeoutMs: Ptr(-1), getenv: noopGetenv}); err == nil || !strings.Contains(err.Error(), "timeoutMs") {
		t.Fatalf("expected a timeoutMs error, got %v", err)
	}
}

func TestResolveConfigRejectsNegativeMaxRetries(t *testing.T) {
	_, err := resolveConfig(Options{MaxRetries: Ptr(-1), getenv: noopGetenv})
	if err == nil || !strings.Contains(err.Error(), "maxRetries") {
		t.Fatalf("expected a maxRetries error, got %v", err)
	}
}

func TestResolveConfigRejectsNonPositiveMaxRetryDelay(t *testing.T) {
	_, err := resolveConfig(Options{MaxRetryDelayMs: Ptr(0), getenv: noopGetenv})
	if err == nil || !strings.Contains(err.Error(), "maxRetryDelayMs") {
		t.Fatalf("expected a maxRetryDelayMs error, got %v", err)
	}
}

// Finding 1 (Node review, class carried over): an explicit environment must
// beat a stray env var, on BOTH hosts, so REST and the socket cannot split.
func TestResolveConfigExplicitEnvironmentBeatsEnvVars(t *testing.T) {
	config := mustResolve(t, Options{Environment: EnvironmentSandbox, getenv: envFrom(map[string]string{
		"HIGH_BASE_URL": "https://openapi.high.live", "HIGH_WS_BASE_URL": "wss://openapi.high.live",
	})})
	if config.baseURL != Environments[EnvironmentSandbox].API {
		t.Errorf("baseURL leaked from env var: %q", config.baseURL)
	}
	if config.wsBaseURL != Environments[EnvironmentSandbox].WS {
		t.Errorf("wsBaseURL leaked from env var: %q", config.wsBaseURL)
	}
}

func TestResolveConfigHighBaseURLAppliesWithoutEnvironment(t *testing.T) {
	config := mustResolve(t, Options{getenv: envFrom(map[string]string{"HIGH_BASE_URL": "http://localhost:9999"})})
	if config.baseURL != "http://localhost:9999" {
		t.Errorf("baseURL = %q", config.baseURL)
	}
}

func TestBuildURLJoinsBaseVersionAndPath(t *testing.T) {
	config := mustResolve(t, Options{})
	if got := buildURL(config, "/orders/list"); got != "https://openapi.high.live/v1/orders/list" {
		t.Errorf("buildURL = %q", got)
	}
}

// Review Focus 1 (Node): a user pasting a URL out of a browser brings a trailing slash.
func TestBuildURLToleratesStraySlashes(t *testing.T) {
	config := mustResolve(t, Options{BaseURL: "https://h.example/", VersionPath: Ptr("/v2/")})
	if got := buildURL(config, "/orders"); got != "https://h.example/v2/orders" {
		t.Errorf("buildURL = %q", got)
	}
	if got := buildURL(config, "orders"); got != "https://h.example/v2/orders" {
		t.Errorf("buildURL = %q", got)
	}
}

func TestBuildURLSupportsPathPrefix(t *testing.T) {
	config := mustResolve(t, Options{BaseURL: "https://h.example/api/"})
	if got := buildURL(config, "/orders"); got != "https://h.example/api/v1/orders" {
		t.Errorf("buildURL = %q", got)
	}
}

func TestBuildURLAllowsEmptyVersionPath(t *testing.T) {
	config := mustResolve(t, Options{VersionPath: Ptr("")})
	if got := buildURL(config, "/orders"); got != "https://openapi.high.live/orders" {
		t.Errorf("buildURL = %q", got)
	}
}
