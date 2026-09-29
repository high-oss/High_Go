# Changelog

All notable changes to this project are documented in this file.

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
