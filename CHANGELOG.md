# Changelog

All notable changes to this project are documented in this file.

## Unreleased

### Added

- `Profile.Get` (`GET /profile`) returns the account holder's identity, demat and
  bank details as `generated.Profile`. PAN, Aadhaar and bank account numbers are
  masked to their last four characters.

## 0.1.0 — 2026-10-24

Types regenerated from `High/sdk/spec` at commit `c1f37de4ec27245288ef9c9f83699c51f29b3b50`.

### Added

- `Scrips.Intraday`, the most recent trading session's candles, columnar and in
  the same shape as `Historical`. Takes `IntradayRequest`: name the instrument
  by `TradingSymbol`, or by `ExchangeSegment` + `ScripCode`.
- `Portfolio.SetStoploss` (`PATCH /portfolio/positions/stoploss`) places or
  amends a stop-loss exit order for an open position. Takes `SetStoplossRequest`
  and returns `generated.PositionRiskOrderResult`.
- `Portfolio.SetTarget` (`PATCH /portfolio/positions/target`) does the same for
  a target (limit) exit, with `SetTargetRequest` taking `TargetPrice`.

### Changed

- `Scrips.Historical` accepts the instrument as `TradingSymbol` or as
  `ExchangeSegment` + `ScripCode`. `TradingSymbol` is no longer required, so it
  is now a `*string` in `HistoricalRequest`: wrap existing values with `Ptr`.
- The generated `HistoricalJSONBodyInterval` constants are renamed with a
  `HistoricalJSONBodyInterval` prefix (for example `N1D` is now
  `HistoricalJSONBodyIntervalN1D`). Code that imports `generated` directly
  must update; the SDK's own API does not expose these constants.

### Removed

- `Logo` is removed from `generated.ScripInfo`. The API no longer returns it.


## 0.0.1 — 2026-09-28

Initial release. Covers 24 of the 27 HIGH Open API operations (`auth`,
`orders`, `portfolio`, `scrips`, `market`) per the SDK shared contract. Types
generated from `High/sdk/spec` at commit `339cdc084dd75a952d9d5cc11636c49dd98d00cf`.

Added `Instruments`, a sixth resource covering the instrument list (the scrip
master) across its five categories — a streaming `Stream` iterator and an
eager `List`, both needing no credentials. Types regenerated from `High/sdk/spec`
at commit `97cc3af3df39c0252b55306d9ce56accdad65c02`.

Added `Feed`, a second, production-only client for the live datafeed socket,
built from the same `Options` and credentials as `Client`. Subscribe by HIGH
scrip key across `SubscribeQuotes`/`SubscribeDepth`/`SubscribeIndices` (and
their `Unsubscribe`/`Snapshot` counterparts); receive merged, typed
`Quote`/`Depth`/`Index` snapshots over `Quotes()`/`Depths()`/`Indices()`.
Reconnects with backoff on a transport failure, re-authenticating and
re-subscribing before resuming delivery; never retries a refused
authentication. Adds this SDK's one other permitted runtime dependency,
[`github.com/coder/websocket`](https://github.com/coder/websocket).
