# relaycore

The relay engine: websockets, subscriptions, the NIP-01 message loop, NIP-42
AUTH, NIP-45 COUNT, NIP-77 sync, NIP-86 management, plus the `blossom` and
`policies` helpers.

**This is our code.** It started as `github.com/fiatjaf/khatru@v0.19.1` (MIT,
see LICENSE) and was adopted wholesale rather than depended on. There is no
upstream to track: nothing here is expected to stay in sync, and updates are a
deliberate choice rather than an obligation.

## Why it was adopted

Two reasons, in order of weight.

**We already had to work around it.** Three of its behaviours were wrong for us
and are patched in `internal/relay`: BUD-09 reports read into a nil body,
BUD-04 mirroring fetched user-supplied URLs with no address checks (an SSRF),
and the NIP-86 decoder panicked on `grantadmin`/`revokeadmin`. Carrying local
fixes against a moving dependency is worse than owning the code.

**NIP-43 needs control of the OK message.** The spec dictates the exact text a
join request is answered with. Upstream replaces the reason for every ephemeral
event with `"broadcasted to N listeners"`, and there is no hook in between —
`WebSocket.conn` is unexported and `WriteJSON` is a concrete method, so the
write cannot be intercepted from outside the package either.

## What we changed

Everything we touched is marked `FORK:` in the source. Two changes:

- **`relay.go` / `handlers.go`** — a new `OverwriteOK` hook, run immediately
  before the OK envelope is written, so the application decides the final
  wording. A relay that registers none behaves exactly as before.
- **`handlers.go`** — the per-REQ context is now built by `newReqContext`, which
  returns its cancel func. Identical behaviour; it just lets `go vet` see that
  the cancel is handed to the listener registry rather than dropped.

Package renamed from `khatru` to `relaycore`, imports rewritten, tests and docs
left behind.

## Pulling in an upstream change

Occasionally worth doing for a security or correctness fix. There is no
automated path; read the upstream diff and port what matters:

    git -C /path/to/khatru log --oneline v0.19.1..

Then apply by hand, keeping the two changes above. The package rename means a
plain patch will not apply.
