# high-openapi

Official Go SDK for the [HIGH Open API](https://openapi.high.live). Typed
against the canonical OpenAPI contract, generated with
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

## Install

```bash
go get github.com/high-oss/High_Go
```

Go 1.23 or newer.

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	highopenapi "github.com/high-oss/High_Go"
)

func main() {
	high, err := highopenapi.New(highopenapi.Options{
		Environment: highopenapi.EnvironmentSandbox, // or EnvironmentProduction (the default)
		APIKey:      os.Getenv("HIGH_API_KEY"),
		AccessToken: os.Getenv("HIGH_ACCESS_TOKEN"),
	})
	if err != nil {
		log.Fatal(err)
	}

	funds, err := high.Portfolio.Funds(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(funds.AvailableBalance)
}
```

See `Example` and `Example_errors` in `example_test.go` for a runnable,
`go test`-checked version of this against a local server.

## Configuration

| Option | Default | Notes |
|---|---|---|
| `Environment` | `EnvironmentProduction` | `EnvironmentProduction` or `EnvironmentSandbox` |
| `BaseURL` | from `Environment` | Overrides the REST host |
| `WSBaseURL` | from `Environment` | Datafeed socket host, reserved for the feed client |
| `VersionPath` | `"v1"` | The segment between host and operation path. `nil` means "not set"; `Ptr("")` is a valid, explicit unversioned host |
| `APIKey` | `HIGH_API_KEY` | Sent as `x-api-key`, for `Auth.GenerateAccessToken` |
| `AccessToken` | `HIGH_ACCESS_TOKEN` | Sent as `Authorization: Bearer` |
| `TimeoutMs` | `30000` | Per request, covering the whole exchange. `nil` means "not set"; must be `> 0` |
| `MaxRetries` | `2` | Idempotent reads only. `nil` means "not set"; must be `>= 0` |
| `MaxRetryDelayMs` | `30000` | Ceiling on one retry delay; a longer `Retry-After` fails fast. `nil` means "not set"; must be `> 0` |
| `LogLevel` | `LogLevelSilent` | `silent`, `error`, `warn`, `info`, `debug` |
| `LogSink` | stderr | Where log lines go |
| `UserAgent` | — | Appended to the SDK's own |
| `HTTPClient` | `http.DefaultClient` | Transport override |
| `InstrumentsAllowedHosts` | the instrument list's CDN host | Hosts the instrument list may be downloaded from; a file whose host is not on this list is refused before any request is made |

`TimeoutMs`, `MaxRetries`, `MaxRetryDelayMs` and `VersionPath` are `*int`/
`*string` rather than plain values, because their zero value is a real,
valid, explicitly-rejected or explicitly-meaningful input (an explicit
`TimeoutMs: 0` must fail construction naming the option, and an explicit
empty `VersionPath` is a legitimate unversioned host) — a bare `int`/`string`
field cannot tell "not set" apart from "set to the zero value". Use the
`Ptr` helper: `highopenapi.Options{TimeoutMs: highopenapi.Ptr(5000)}`.

Host resolution order: explicit `BaseURL`, then explicit `Environment`, then
`HIGH_BASE_URL`, then `HIGH_ENVIRONMENT`, then production. An unknown
environment returns an error from `New`, naming the valid values — never at
request time. An explicit `Environment` also suppresses `HIGH_BASE_URL` /
`HIGH_WS_BASE_URL`, so a value left in a shell profile cannot silently route
a client written as `Environment: EnvironmentSandbox` at production.

Sandbox and production issue **separate API keys**. Switching environment
means switching credentials too; the SDK does not rewrite them for you.

## Authentication

The SDK covers the **TOTP flow**, which is one call:

```go
high, _ := highopenapi.New(highopenapi.Options{APIKey: os.Getenv("HIGH_API_KEY")})

token, err := high.Auth.GenerateAccessToken(context.Background(), "C1", "123456")
if err != nil {
	log.Fatal(err)
}

// Use it for everything else.
trading, _ := highopenapi.New(highopenapi.Options{AccessToken: token.AccessToken})
```

A HIGH access token lasts 24 hours. The SDK attaches whatever you give it and
**does not refresh** — minting the next one needs a fresh TOTP, so the timing
is yours to choose. Configuration is settled at construction, so a new token
means a new client.

### What this SDK does not do

It wraps the TOTP endpoint and nothing else from the auth area. The redirect
consent flow (`/auth/generate-consent`, `/auth/login`, `/auth/consume-consent`)
and token introspection (`/auth/validate-token`) are not part of the SDK; if
you need them, call those endpoints directly.

## Errors

Every failure is an `*APIError` — the one exported error type, recoverable
with `errors.As`. Transport failures and timeouts carry `Status: 0`. No raw
transport error and no JSON decode error escapes the SDK.

```go
import "errors"

_, err := high.Orders.Place(ctx, order)
var apiErr *highopenapi.APIError
if errors.As(err, &apiErr) {
	apiErr.Status    // HTTP status, or 0 for transport failures
	apiErr.Code      // e.g. "ORDER_REJECTED" — see ErrorCodes
	apiErr.RequestID // quote this in support tickets
	apiErr.Messages  // validation errors return several
	apiErr.Body      // the parsed payload, for a non-standard error
}
```

`ErrorCodes` holds the documented catalogue. The gateway can return codes
outside it, so `Code` is a plain `string` — compare against the constants,
but do not assume the set is closed.

## Retries

Retried: `GET` and `HEAD` only, on 429, 500, 502, 503 and 504, honouring
`Retry-After` in both its seconds and HTTP-date forms, with exponential
backoff and jitter otherwise. A `Retry-After` longer than `MaxRetryDelayMs`
(30s by default) is not waited out — the SDK gives up and returns an error,
rather than blocking your call inside a retry it cannot interrupt.

Never retried: `POST`, `PATCH` and `DELETE`. A retried `Orders.Place` would be
a duplicate order, and a retried square-off would be a second square-off.

## Logging

Off by default. Each level prints itself and everything more severe.

```go
high, _ := highopenapi.New(highopenapi.Options{AccessToken: token, LogLevel: highopenapi.LogLevelDebug})
// 2026-09-28T17:05:12.345Z DEBUG HIGH -> POST https://openapi.high.live/v1/orders
//   map[body:map[quantity:1 tradeSide:B tradingSymbol:RELIANCE-EQ …]]
// 2026-09-28T17:05:12.488Z DEBUG HIGH <- 200 in 143ms https://openapi.high.live/v1/orders
//   map[body:map[error: orderId:2609250000123456] requestId:a1b2c3]
```

| Level | What it prints |
|---|---|
| `error` | API error responses (status, code, requestId), timeouts, transport failures |
| `warn` | Each retry, and giving up when `Retry-After` exceeds the ceiling |
| `info` | One line per request: method and URL |
| `debug` | The above plus request and response bodies, and response timing |

Every line is prefixed with an ISO-8601 timestamp and the level.

Credentials never reach the log. Headers are not logged at all, and the
`tOtp`, `apiKey`, `accessToken`, `tokenId` and `stepToken` query parameters
are replaced with `REDACTED` — so debug logs are safe to ship to an
aggregator. Pass `LogSink` (anything satisfying the `LogSink` interface) to
route lines somewhere other than stderr.

## Cancellation

Every method takes a `context.Context` as its first argument — Go's
cancellation primitive, and the mechanism the whole-exchange timeout and the
retry policy are both built on. A caller's cancellation is surfaced
unchanged: never converted into a timeout, never retried, and honoured
during a retry's sleep as well as during the request itself.

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
quotes, err := high.Scrips.Quotes(ctx, highopenapi.QuotesRequest{
	Symbols: highopenapi.Ptr([]string{"RELIANCE-EQ"}),
})
```

## Resources

24 operations across five namespaces.

| Namespace | Methods |
|---|---|
| `Auth` | `GenerateAccessToken` |
| `Orders` | `Place` · `Modify` · `Get` · `Cancel` · `List` · `Trades` · `TradesFor` · `Charges` · `Margin` |
| `Portfolio` | `Positions` · `Holdings` · `Funds` · `ConvertPosition` · `ExitAllPositions` · `ExitPosition` |
| `Scrips` | `Quotes` · `Ohlc` · `Depth` · `Expiries` · `FutureData` · `Historical` · `OptionChain` |
| `Market` | `Status` |

Each method returns the response's `data` — the `{requestId, data}` envelope
is unwrapped for you, and `RequestID` reaches you on the error.

A few shapes worth knowing, because they are not what you might guess:

```go
// Quotes and Ohlc return a MAP keyed by trading symbol, not a slice.
quotes, _ := high.Scrips.Quotes(ctx, highopenapi.QuotesRequest{Symbols: highopenapi.Ptr([]string{"RELIANCE-EQ"})})
quotes["RELIANCE-EQ"].LTP

// Historical returns COLUMNAR arrays — index i across them is one candle —
// and takes FromTime/ToTime as epoch seconds (int).
candles, _ := high.Scrips.Historical(ctx, highopenapi.HistoricalRequest{
	TradingSymbol: "RELIANCE-EQ", Interval: "1D",
	FromTime: 1789929000, ToTime: 1790330400,
})
candles.Close[0] // not candles[0].Close

// Positions and Holdings both have a Snapshot, with DIFFERENT shapes.
positions, _ := high.Portfolio.Positions(ctx)
positions.Snapshot.TotalPL
holdings, _ := high.Portfolio.Holdings(ctx)
holdings.Snapshot.Investment

// scripCode is a STRING on Order/Trade, an INT on ScripInfo — the generated
// types describe the API as it is; this SDK does not normalise it.

// Order.Error is nullable (*string), and Order.TradingSymbol may be absent
// (*string) when the instrument is not in the local master.

// An OptionLeg may be entirely empty (every field is optional), and its
// Touchline may be nil when the quote feed has no entry for that strike.
```

## Instrument list

`Instruments` fetches the scrip master — every scrip HIGH knows, per
category — as the `Instrument` type. It needs no credentials at all: no
`APIKey`, no `AccessToken`. Five categories are published:

| Category | Constant | Covers |
|---|---|---|
| `all` | `InstrumentAll` | Every scrip |
| `equity` | `InstrumentEquity` | NSE/BSE cash |
| `derivatives` | `InstrumentDerivatives` | NSE/BSE futures and options |
| `commodity` | `InstrumentCommodity` | MCX futures, options and spot |
| `etfs` | `InstrumentEtfs` | NSE/BSE ETFs |

The streaming form is the primary API — `all` and `derivatives` run into the
tens of megabytes, so reading the whole thing into memory first is
unfriendly by default:

```go
for instrument, err := range high.Instruments.Stream(ctx, highopenapi.InstrumentEquity) {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(instrument.TradingSymbol, instrument.ScripCode)
}
```

`List` is the eager form, for when you genuinely want every row in memory at once:

```go
rows, err := high.Instruments.List(ctx, highopenapi.InstrumentEquity)
```

Both take `context.Context` first; breaking out of `Stream`'s loop early
closes the underlying connection cleanly. The two largest files can take
longer than the client's default 30s `TimeoutMs` to download on a slow
connection — that setting still governs ordinary operations, but a fetch
here is bounded only by the context you pass, so give it a longer deadline
rather than raising the client-wide default:

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
defer cancel()
rows, err := high.Instruments.List(ctx, highopenapi.InstrumentAll)
```

Each row is an `Instrument` with all 17 columns:

| Field | Type | Notes |
|---|---|---|
| `Exchange` | `string` | `NSE`, `BSE` or `MCX` |
| `Segment` | `string` | |
| `InstrumentType` | `string` | e.g. `EQUITY`, `IDX`, `FUTSTK`, `OPTSTK` |
| `TradingSymbol` | `string` | The symbol every other operation takes |
| `ScripKey` | `string` | e.g. `NSE@2885` |
| `ISIN` | `string` | Blank where not applicable |
| `ScripCode` | `int` | Broker-assigned numeric ID |
| `Symbol` | `string` | |
| `Name` | `string` | |
| `GroupSeries` | `string` | |
| `HasFnO` | `int` | `0`/`1` |
| `UnderlyingSymbol` | `string` | Blank for a non-derivative |
| `Expiry` | `string` | Date; blank outside derivatives |
| `OptionType` | `string` | Blank outside options |
| `StrikePrice` | `*float64` | `nil` when blank — see below |
| `PriceTick` | `int` | **Not normalised** — see below |
| `LotSize` | `int` | |

Every field is its plain zero value when the CSV cell is blank, except
`StrikePrice`. `ScripCode`, `HasFnO`, `PriceTick` and `LotSize` never take a
genuine zero in real data — no real broker token, lot size or price tick is
`0`, and `HasFnO`'s own false state *is* zero — so a blank cell and a real
value can never be confused for these four. `StrikePrice` is different:
every non-option row has no strike at all, and a caller might reasonably
compare a strike against `0` somewhere downstream, so leaving it a plain
`float64` would let a blank cell silently read as "strike price zero"
instead of "not applicable". It is `*float64`: `nil` means the cell was
blank.

`PriceTick` is carried exactly as published, unnormalised — its scale
(paise, rupees, or something else) varies by segment, and rescaling it here
would be a silent pricing bug baked into every caller.

The list is rebuilt once each trading morning and does not change
intraday — cache it for the day rather than re-fetching on every call.

## Live datafeed

Not in this release. `Options.WSBaseURL` is resolved onto the client's
config for a future `Feed` client to reuse, alongside the same credentials —
matching the REST client's config rather than introducing a second config
surface. No socket code ships here.

## Regenerating

Types are generated from the canonical spec at the commit pinned in
`spec.lock.json`, using oapi-codegen:

```bash
# From ../spec, at the pinned commit:
git -C ../spec show <commit>:openapi.json > generated/openapi.json
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -config generated/config.yaml generated/openapi.json
```

`generated/` is committed and never hand-edited. To move to a newer
contract, update `spec.lock.json`, regenerate, and commit both. The generated
types are the authority: when a hand-written payload disagrees with them,
the payload is wrong, not the generated type.

## Development

Go is not installed on the reference machine this SDK was built on; every
command below runs in Docker.

```bash
docker run --rm -v "$(pwd):/w" -v go-mod-cache:/go/pkg/mod -w /w golang:1.23 \
  bash -c "go build ./... && go vet ./... && go test ./..."
```

Tests run against a real local server (`net/http/httptest`), never a
fabricated transport, so a change in how the SDK builds requests is caught
rather than asserted around.

## Licence

MIT. See [LICENSE](./LICENSE).
