# nostrel

*English · [한국어](README-KR.md)*

A Nostr relay that runs its write whitelist on lightning payments. Web panel included, one binary and one SQLite file.

- Relay engine: `internal/relaycore` (started from khatru, absorbed)
- Payments: LNbits, NWC (NIP-47), and a mock for local testing
- Billing: admission fee (once) + subscription (per period) + storage (MB) prepaid — events and uploaded files draw on the same quota
- On sale: relay access, **NIP-05 identifiers** (priced per domain, per period), **groups** (several pubkeys sharing one quota)
- Admin auth: NIP-98 signed login, with a password fallback
- Media: Blossom (BUD-01/02) + NIP-96

## NIP support checklist

A checked box means the relay implements it at the protocol level. *Conditional* items only switch on once their configuration is in place.

- [x] **NIP-01** Basic protocol
- [x] **NIP-04** Encrypted DMs — the relay enforces who may read them
- [x] **NIP-05** DNS-based identifiers — names sold per domain *(conditional)*
- [x] **NIP-09** Event deletion — refunds the quota
- [x] **NIP-11** Relay information document — generated live from settings
- [x] **NIP-13** Proof of Work *(conditional: panel `min_pow > 0`)*
- [x] **NIP-17** Private DMs *(conditional: incoming DMs accepted, on by default)*
- [x] **NIP-40** Event expiration — GC reclaims the quota afterwards
- [x] **NIP-42** AUTH
- [x] **NIP-43** Relay access and invite codes *(conditional: `RELAY_SECRET_KEY`)*
- [x] **NIP-44** Encryption (v2)
- [x] **NIP-45** COUNT
- [x] **NIP-46** Remote signing (bunker) — public panel login
- [x] **NIP-50** Search — including the `domain:` extension
- [x] **NIP-56** Reporting — feeds the NIP-86 moderation queue
- [x] **NIP-57** Lightning zap receipts *(conditional: off by default)*
- [x] **NIP-58** Badge awards *(conditional: off by default)*
- [x] **NIP-59** Gift wrap — the relay enforces who may read them, and accepts them for a customer *(conditional: on by default)*
- [x] **NIP-62** Right to vanish
- [x] **NIP-70** Protected events
- [x] **NIP-77** Negentropy sync
- [x] **NIP-86** Relay management API — every method
- [x] **NIP-96** HTTP file storage *(conditional: file storage available)*
- [x] **NIP-98** HTTP auth — panel, NIP-96, NIP-86
- [x] **NIP-5A** Static site hosting (nsites) *(conditional: nsite domains configured)*
- [ ] **NIP-29** Relay-based groups — not supported; its permission model collides with a paid whitelist ([below](#gift-wrap-nip-59))

Blossom BUD-01/02/04/05/06/09 are implemented too — see [media hosting](#media-hosting).

**NIPs with nothing for a relay to implement** — ordinary events an author writes under their own pubkey, stored and served as-is once they clear the whitelist: 02, 07, 19, 23, 25, 28, 51, 65, 75 (zap goals), 7D (forum threads), 84 (highlights), 85 (trust assertions), B7 (Blossom server lists), and so on. To block a particular kind, use NIP-86 `allowkind`/`disallowkind`. NIP-47 (NWC) is used by the relay **as a client**, to talk to a payment backend.

### Details

| NIP | Feature | Notes |
|---|---|---|
| 01 | Basic protocol | EVENT/REQ/CLOSE, subscriptions |
| 04 | Encrypted DMs | kind 4 is readable only by the parties |
| 05 | DNS-based identifiers | `/.well-known/nostr.json`, names sold per domain |
| 09 | Event deletion | deleting refunds the quota |
| 11 | Relay information document | fees, limits and policy generated live from settings |
| 13 | Proof of Work | on when the panel's `min_pow` is set; committed difficulty is checked |
| 17 | Private DMs | rides on NIP-59 gift wrap; a wrap addressed to a customer is accepted and billed to them |
| 40 | Event expiration | dropped from queries the moment it expires; GC reclaims disk and quota after |
| 42 | AUTH | DM reads, the panel's `read_auth_required`, NIP-43 requests |
| 43 | Relay access, invite codes | when `RELAY_SECRET_KEY` is set (see below) |
| 44 | Encryption (v2) | used by the panel's NIP-46 traffic |
| 45 | COUNT | |
| 46 | Remote signing (bunker) | public panel login; the browser talks to the signer directly |
| 50 | Search | substring match on `content` plus a `domain:` extension |
| 56 | Reporting | kind 1984 → NIP-86 moderation queue |
| 57 | Lightning zaps | accepts kind 9735 receipts (off by default, see below) |
| 58 | Badges | accepts kind 8 awards (off by default, see below) |
| 59 | Gift wrap | kinds 1059 and 13 are readable only by sender and recipient; backdating allowed; the throwaway signing key needs no account |
| 62 | Right to vanish | kind 62 → deletes that pubkey's events and files, and blocks re-upload |
| 5A | Static sites (nsites) | hosts `<name>.<domain>` off a NIP-05 name sold here |
| 70 | Protected events | the `-` tag |
| 77 | Negentropy sync | verified with `nak sync` |
| 86 | Relay management API | every method, below |
| 96 | HTTP file storage | `/.well-known/nostr/nip96.json` |
| 98 | HTTP auth | panel, NIP-96, NIP-86 |

NIP-11 `supported_nips` reports **the actual state**. The always-on set (01, 04, 09, 11, 40, 42, 45, 50, 56, 62, 70, 77, 86, 98) is fixed; the rest come and go with configuration:

| NIP | Condition |
|---|---|
| 13 | panel `min_pow > 0` |
| 43 | `RELAY_SECRET_KEY` is set |
| 05 | at least one domain on sale |
| 17 / 59 | **Accept direct messages from outside** (on by default) |
| 57 / 58 | each third-party event toggle |
| 96 | file storage is available |

04, 17 and 59 are on that list not because the events are merely stored, but because **the relay enforces who can read them** — and, for 17 and 59, who may write one to a customer.

### Events third parties write to or about a customer (NIP-04, NIP-17, NIP-57, NIP-58)

These are the exception. The author is not the customer, but the event is **about** a customer, or addressed **to** one — left alone, every one of them would be rejected.

| Event | Author | Setting |
|---|---|---|
| Gift wrap (kind 1059), kind 4 DM | a throwaway key (NIP-59), or a sender without an account | **Accept direct messages from outside** *(on)* |
| Zap receipt (kind 9735) | the LNURL server that took the payment | **Allow third-party zap receipts** |
| Badge award (kind 8) | the badge issuer | **Allow third-party badge awards** |

Once on, an event only gets through when:

- **All**: the **first** pubkey among the candidates that has an account (or a group) here becomes the payer, and the event has to clear that person's write permission and quota. If none of them is a customer, it is rejected.
- **Direct messages**: exactly one `p` tag and a non-empty `content`. The candidate list starts with the **author**, so a customer sending a message pays for it themselves; only a message from outside is billed to the recipient, and the switch only ever applies to that case.
- **Zap receipts**: exactly one `p`. A `bolt11` tag is required. `description` must be a kind 9734 zap request with a **valid id and signature**, whose `p` matches the recipient.
- **Badge awards**: the `a` tag must read `30009:<author pubkey>:<slug>` — the issuer has to point at **their own** badge definition, so nobody can award somebody else's badge. Multiple `p` tags are allowed (per NIP-58).

The quota is billed **to the recipient**, not to whoever uploaded it. The relay cannot tell whether the payment or the award actually happened, so turning these on means a stranger can spend a customer's quota — which is why zaps and badges default to off, and why abusive authors get banned through NIP-86. 57 and 58 appear in NIP-11 `supported_nips` only while the respective toggle is on.

**Direct messages are the one that defaults to on.** A NIP-59 gift wrap is *always* signed by a key made for that single message, so with the switch off NIP-17 does not work at all — not even between two paying customers — and 17 and 59 drop out of `supported_nips` to say so. The cost of leaving it on is that anyone can spend a customer's storage by messaging them; ban them with NIP-86, or turn the switch off and keep kind 4 between customers only.

The remaining badge kinds (30009 definitions, 10008 profile badges, 30008 badge sets) are ordinary self-authored events and work regardless of these settings.

### Invite codes (NIP-43)

The way in without paying: beta invites, invites for friends, event coupons. It **does not replace payment, it waives the admission fee** — NIP-43 itself is neutral about price.

**Turning it on**: put a 64-character hex private key in `RELAY_SECRET_KEY`. The relay has to be able to sign under its own name for NIP-43 to switch on; without it everything is disabled and 43 never appears in `supported_nips`.

```bash
openssl rand -hex 32   # RELAY_SECRET_KEY
```

**That key is the relay's identity.** Leak it and somebody else can forge membership lists in our name. That is why it never goes into the database and is not editable from the panel — environment only. When the panel's operator pubkey is empty, NIP-11 falls back to the pubkey of this key.

| Flow | How |
|---|---|
| Admin issues a code | panel `Invite codes` — set period, quota, number of uses, expiry |
| Client asks for a code | subscribe to `kind 28935`. Only when **auto invite** is on; each request gets a fresh single-use code (10-minute expiry) |
| Join with a code | `kind 28934` + a `claim` tag → account created, admission fee waived |
| Leave | `kind 28936` → access cut off |
| Relay publishes | `33534` roles (member, admin), `13534` membership (up to 1000 people), `8000`/`8001` join and leave notices |

On the web, sign in and drop the code into the `Invite codes` card on the public panel (`POST /api/invite/claim`, NIP-98 signed). Same result, no wallet needed.

**Leaving does not delete data.** It drops the status to `banned`, which only stops writes — a paying user's events must not be erased on the strength of a "cut off my access" request. Full erasure is NIP-62.

**Concurrency**: several people can claim the same code at once; expiry, remaining uses and duplicates are all handled inside one transaction, so it never over-issues. The same person cannot burn two uses with the same code either.

**AUTH is required.** NIP-43 makes the NIP-70 `-` tag mandatory on requests, and protected events must go through NIP-42 auth. So a client has to finish AUTH before a join or leave request — the relay sends the `AUTH` challenge first. The AUTH event's `relay` tag has to match `SERVICE_URL`.

The response strings are the spec's own examples:

```
["OK", <id>, false, "restricted: that is an invalid invite code."]
["OK", <id>, true,  "info: welcome to ws://localhost:3399!"]
["OK", <id>, true,  "duplicate: you are already a member of this relay."]
```

We can pick the success string only because the relay engine lives in `internal/relaycore`. Upstream (khatru) overwrites the OK reason for ephemeral events with `"broadcasted to N listeners"` unconditionally, and `WebSocket.conn` is private, so it cannot be intercepted from outside the package either. We added an `OverwriteOK` hook — see `internal/relaycore/ORIGIN.md`.

### Right to vanish (NIP-62)

When a user sends kind 62 we forget that pubkey **whether or not they have an account**. The spec is explicit that paid and restricted relays are no exception.

The request is only processed when its `relay` tag carries this relay's `SERVICE_URL` (ignoring scheme and trailing slash) or `ALL_RELAYS`. What happens:

1. Every event at or **before** the request's `created_at` is deleted
2. Ownership of uploaded files is released — bytes survive if somebody else uploaded the same file, but this user's share of the quota is refunded
3. Quota is recomputed for what was deleted
4. A **cutoff is recorded** so the same events cannot come back (the spec's "MUST ensure the deleted events cannot be re-broadcasted")

New events after the cutoff work normally — you can keep using the relay after asking to be forgotten. `store.Unvanish` exists to undo a mistake, but deleted events do not come back.

**Payment records (`payments`) are not erased.** They are accounting data with a different retention policy. Erasing them is a separate decision.

### Static site hosting (NIP-5A nsites)

Put **nsite hosting domains** into the panel's `Pricing · relay info`, and a NIP-05 name sold here becomes a site address:

```
alice@sites.example.com  →  https://alice.sites.example.com
```

The user uploads files over Blossom and publishes a kind 15128 manifest (`["path", "/index.html", "<sha256>"]`) to this relay. Since we are both the relay and the Blossom server, **no other relay and no `server` hint is ever queried** — the manifest and the files are local.

- `/` and extensionless paths get `index.html` appended
- Missing paths fall back to the site's `/404.html`, then to a 404
- Blobs are content-addressed, so `Cache-Control: max-age=3600` + `X-Content-Type-Options: nosniff`

For DNS, point `*.sites.example.com` at this server. The panel's own host is unaffected (routing is by Host alone).

### Search extension (NIP-50)

`domain:<domain>` is supported. It **looks up the NIP-05 names sold here directly**, which makes it more accurate than relays that trust profile metadata.

```
["REQ","x",{"search":"domain:example.com cats","kinds":[1]}]
```

`domain:` turns into an author filter and only the remaining words are used for full-text search. If `authors` is already present it is an intersection, and if that domain has no valid names at all the result is **empty** (the filter is not silently ignored). Expired names do not count.

`include:spam`, `language:`, `sentiment:` and `nsfw:` are **ignored** as the spec requires — stripped from the query, no effect on results. Without stripping, `nsfw:false cats` would be searched literally and match nothing.

### Gift wrap (NIP-59)

Kinds 1059 and 13 are readable only by sender and recipient (NIP-42 auth required regardless of `read_auth_required`). NIP-17 private DMs ride on top of this.

Gift wraps randomize `created_at` **up to two days into the past** to hide the real send time. So the `created_at` past limit is deliberately not applied to kind 1059 — applying it would silently break private DMs the moment that setting is turned on.

The wrapping key is thrown away after one message, so it can never be a customer. A wrap naming a customer in its single `p` tag is therefore accepted on their behalf and billed to them — see [events third parties write](#events-third-parties-write-to-or-about-a-customer-nip-04-nip-17-nip-57-nip-58).

**Not supported**: NIP-29 (relay-based groups). It needs a separate framework ([relay29](https://github.com/fiatjaf/relay29)) and the groups carry their own permission model, which collides with a paid whitelist. If you need it, run a dedicated instance for groups.

## Quick start

```bash
go build -o nostrel ./cmd/nostrel        # needs CGO (go-sqlite3)
cp .env.example .env                     # fill in the values
export $(grep -v '^#' .env | xargs)
./nostrel
```

The relay engine is not an external dependency, it lives in `internal/relaycore`. It started from khatru but has been absorbed into our own code, and there is no upstream to follow — background in `internal/relaycore/ORIGIN.md`.

To watch the flow locally without payments, just start it — a fresh database ships with the `mock` payment backend, which treats every invoice as paid the moment it is issued:

```bash
PANEL_URL=http://localhost:3334 ./nostrel
```

**Never leave mock on in production.** Switch it under `Admin → Payment backend` before taking money; until you do, every start logs a warning.

### Configuration: what lives where

The environment holds only what has to be known before the database is open and an admin can log in. **Everything else is edited in the admin panel and stored in the database**, so it changes without a restart.

| Variable | What it is |
|---|---|
| `RELAY_PORT` | port to listen on (all interfaces) |
| `DB_PATH` | SQLite file; events, accounts and settings all live in it |
| `PANEL_URL` | public https address of the panel; NIP-98 logins are checked against it |
| `SERVICE_URL` | public wss:// address advertised to clients |
| `RELAY_SECRET_KEY` | the relay's NIP-43 signing key — its identity, never stored in the database |
| `ADMIN_PUBKEYS` | comma separated hex pubkeys allowed into the panel and NIP-86 |
| `ADMIN_PASSWORD_HASH` | fallback password login — a **bcrypt** hash (`$2a$10$…`), never the password itself; print one with `nostrel hash-password '…'` |
| `SESSION_SECRET` | signs password session cookies; without it sessions die on restart |

Panel-owned, in `Admin`: relay name, description, icon, banner, operator contact and pubkey, theme, retention, countries, languages, topics, prices, NIP-05 premium tiers, NIP-46 relays, nsite domains, auto-invite, kind policy, incoming DM acceptance, third-party zap/badge acceptance, `read_auth_required`, `min_pow`, the `created_at` limits, the payment backend and its credentials, and the media backend including `max_blob_size_mb`.

> **Upgrading from a build that read these from the environment:** the moved settings are *not* migrated out of `.env` — they come up on their defaults and are then owned by the panel. If you were running with `READ_AUTH_REQUIRED=true`, `MIN_POW`, custom `created_at` limits, a relay name or a payment backend in the environment, set them again under `Admin` on the first start. A private relay that is not reconfigured comes back **readable by anyone**.

Docker:

```bash
docker build -t nostrel .
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env nostrel
```

### docker compose

`docker-compose.yml` ships with the repository. It defaults to the GHCR `latest` image and a named volume (`nostrel-data`).

```bash
cp .env.example .env      # fill it in (PANEL_URL, SERVICE_URL and ADMIN_PUBKEYS are required)
docker compose up -d
docker compose logs -f
```

The database (`/data/nostrel.db`) and uploaded files (`/data/blobs`) share one volume, so tearing the container down and recreating it picks up exactly where it left off as long as the volume survives.

| What you want | Command |
|---|---|
| Update to the newest image | `docker compose pull && docker compose up -d` |
| Stop (keep the data) | `docker compose down` |
| Stop and delete the data | `docker compose down -v` |
| Hash an admin password | `docker compose run --rm nostrel hash-password 'password'` |
| Recompute usage | `docker compose run --rm nostrel recompute-usage` |
| Build from local source | swap the `image:` line for `build: .`, then `docker compose up -d --build` |

To pin a version, change `image:` to something like `ghcr.io/pixelzx0/nostrel:rc-1.2.3` — recommended in production, for the reason given under the tag list below.

If you changed `RELAY_PORT` in `.env`, change the container side of `ports:` to match. Behind a reverse proxy, narrow `ports:` to `- "127.0.0.1:3334:3334"` and have the proxy pass the `Host` header and the websocket upgrade through untouched — NIP-05 domains and nsite hosting route on `Host`.

If you would rather bind-mount a host directory (`- ./data:/data`), the image runs as uid 10001, so prepare it first: `mkdir -p data && sudo chown 10001:10001 data`.

If you would rather not build at all, use the GHCR images.

| What you do | Tags that move |
|---|---|
| push to `main` | `latest-test`, `test-2026-08-06-150102` (UTC) |
| push a `v1.2.3` tag (including creating a release) | `latest`, `latest-rc`, `rc-1.2.3` |

```bash
# newest release
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel

# newest development build
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel:latest-test

# production: pin the version
docker run -p 3334:3334 -v $PWD/data:/data --env-file .env \
  ghcr.io/pixelzx0/nostrel:rc-1.2.3
```

`latest` moves **only when a release tag goes up** — pushing to `main` does not touch it, so nobody who pulls without a tag ends up on a development build. Still, pin `rc-<version>` in production: a moving tag gives you nothing to roll back to.

`test-<timestamp>` is unique per build. To identify the commit, read the image label (`org.opencontainers.image.revision`):

```bash
docker buildx imagetools inspect ghcr.io/pixelzx0/nostrel:latest-test --format '{{json .Provenance}}'
```

`.github/workflows/docker.yml` builds `linux/amd64`, `linux/arm64` and `linux/arm/v7` and merges them into one manifest. amd64 and arm64 are built **natively** on runners of the matching architecture — running cgo (go-sqlite3) under QEMU takes minutes. Only 32-bit ARM is emulated; drop that row from the matrix if you do not need it (`linux/386` can be added the same way). `go vet` and `go test` have to pass before anything is published.

## How it works

1. The user connects a pubkey in the panel (NIP-07) and picks a period and a quota.
2. `POST /api/order` → a lightning invoice is issued and recorded as pending in the `payments` table.
3. When it is paid, either the LNbits webhook or the 30-second poller notices. **The webhook payload is not trusted** — the payment backend is queried again before anything is granted.
4. The grant happens in the same transaction as the `pending → paid` transition, so double-crediting is impossible.
5. From then on `RejectEvent` checks the whitelist, the expiry and the quota on every incoming event.

Rejection messages:

| Situation | What the client is told |
|---|---|
| Unregistered pubkey | `restricted: not whitelisted — <PANEL_URL>` |
| Subscription expired | `restricted: subscription expired — <PANEL_URL>` |
| Out of quota | `blocked: storage quota exceeded — <PANEL_URL>` |
| Banned | `blocked: this pubkey is banned — <PANEL_URL>` |
| Group subscription expired | `restricted: the group subscription expired — <PANEL_URL>` |
| Group quota exhausted | `blocked: the group storage quota is exhausted — <PANEL_URL>` |

## Storage billing

One event costs its own JSON byte count. On save it is recorded in `usage_events` and added to `accounts.used_bytes`. Deleting gives it back, and replacing a replaceable event cleans up the old row. If you suspect drift:

```bash
./nostrel recompute-usage
```

Every subscription renewal **accumulates** another `included_mb` (a prepaid quota model). To reset it on renewal instead, change the `quota_bytes` update in `grant` (`internal/billing/billing.go`) to an assignment.

## Groups (shared quota)

Several pubkeys share one quota pool. A group buys subscription time and storage like a personal account does, and invites members with no seat limit.

**Only storage is shared.** Which pool an event comes out of is decided per event:

1. If the author's own account is active, unexpired and has quota left → charge the **personal** quota
2. Otherwise, if the group is active, unexpired and has quota left → charge the **group** quota
3. Otherwise, reject

Personal comes first so that a member who already bought their own subscription does not waste it. Which pool paid is recorded in `usage_events.group_id`, and deleting an event refunds **whoever paid**.

A pubkey belongs to at most one group (adding it to another removes it from the previous one). The constraint exists so there is always exactly one answer to "which pool gets billed".

- Buying: name a group in the panel's `Groups` card and get an invoice. If a group already exists, the same button extends and tops it up. Groups carry no admission fee.
- Member management: by the **owner**, signing with NIP-07 (`/api/group/{id}/members/...`), or by an admin from the panel.
- A group subscription alone is enough to write to the relay. No personal account required.

## Login (public panel)

Nothing has to be handed a key — only a pubkey has to be established — so the public panel offers several routes.

| Method | How it works |
|---|---|
| NIP-07 extension | signing is delegated to a browser extension such as Alby or nos2x |
| **NIP-46 bunker** | paste the `bunker://…` string your wallet gave you |
| **NIP-46 nostrconnect** | the panel generates a QR / connection string and the wallet app scans it |
| Paste a pubkey | read-only; group member management is unavailable |

**NIP-46 is handled entirely in the browser.** If the relay held the session with the signer, the operator could ask a user's signer for arbitrary signatures. Instead the page makes a throwaway key, exchanges kind 24133 events with the signer (NIP-44 encrypted, NIP-04 replies accepted too), and asks for `sign_event` only at the moment it is needed. The user's key never reaches the server.

Encryption and signing are handled by vendored [@noble](https://github.com/paulmillr/noble-curves) modules (`web/static/vendor/`); only the NIP layer above them lives in `web/static/nostr.js`. There are no external CDN requests.

**A relay setting is required.** A nostrconnect invitation has to travel over a relay the signer can reach, and this relay is a whitelist — it will not accept events from a pubkey without an account. So a separate relay is used: **NIP-46 login relays** under `Pricing · relay info` (default `wss://relay.nsec.app,wss://nos.lol`, comma separated, works if any one of them connects). A `bunker://` string carries its own relay and is unaffected by this setting.

## Selling NIP-05 identifiers

Identifiers of the form `name@domain` are sold per period. One server can serve many domains at once, and which domain a request is for is decided by the **Host header**.

```bash
curl -H 'Host: example.com' 'https://relay.example.com/.well-known/nostr.json?name=bob'
# {"names":{"bob":"<hex>"},"relays":{"<hex>":["wss://relay.example.com"]}}
```

- Only the requested name is returned. Serving the whole list would publish the customer roster.
- It is queried from browsers, so `Access-Control-Allow-Origin: *` is always set.
- On expiry a name disappears from responses, and the poller removes the row so it goes back on sale. Re-buying stacks on top of the time left.

**Adding a domain**: in the panel's `NIP-05 domains`, enter the domain, the price (sats) and the period (days). Point that domain's DNS at this server and make sure the reverse proxy passes the `Host` header through (caddy does by default; nginx needs `proxy_set_header Host $host;`).

**Name policy**:

- The format follows NIP-05: `[a-z0-9_.-]`, up to 30 characters. Input is normalized to lowercase.
- Block list: register names like `admin` or `support` as unsellable across every domain. Names already sold keep working (reclaiming them is an explicit delete).
- Short-name premium: a `length:multiplier` list in the `Pricing` section. `1:20,2:10,3:5` prices a one-character name at 20× the domain price. Leave it empty for no premium.
- An admin can issue a name directly without payment, and a period of 0 makes it permanent.

**Concurrent purchases**: the name is reserved the moment the invoice is issued, for as long as the invoice lives (one hour). So two people cannot both pay for the same name and have only one get it — the second order is rejected with a 409.

A name can be bought without a relay account. Buying a name does not create an account row, so the admission fee is still owed if you join the relay later.

## Configuring the payment backend

Pick LNbits / NWC / mock and enter the credentials under `Admin → Payment backend`. **Saving applies from the next invoice, with no restart.** There is no environment variable for this: a fresh install starts on `mock`, and the startup log says so on every boot until a real backend is configured.

- **Test connection** button: for LNbits it queries the wallet (`GET /api/v1/wallet`), for NWC it calls `get_info`, verifying credentials and permissions. No money moves. You can test before saving.
- **Secret handling**: the invoice key and the NWC connection string stay on the server, and the API returns only the last four characters (`••••1234`). Leave the field alone and saving keeps the existing value.
- Pricing (admission, subscription, period, included storage, per-MB rate) plus the relay name, description and icon are edited on the same screen and land in NIP-11 immediately.
- A configuration no backend can be built from (LNbits with no URL, say) is rejected at save time.

## Media hosting

Uploads draw on **the same quota as events**, so a user spends the MB they bought on posts and files together. Going over returns a 402. The per-file ceiling is the panel's `max_blob_size_mb` (25 MB by default).

### Where files are stored

Chosen under `Admin → File storage`. Applies to new uploads immediately, **with no restart**.

| Backend | Settings | Notes |
|---|---|---|
| Built-in | storage path | server disk; the default, `./blobs` until the panel says otherwise |
| S3-compatible | endpoint, bucket, region, prefix, keys | AWS S3, MinIO, Cloudflare R2, Backblaze B2, … |

- **Test connection**: writes, reads back and deletes a probe object, so credentials and permissions are actually verified.
- **The secret key** is stored server-side only and the API returns just the last four characters. Leave it alone to keep the existing value.
- **Public URL** (optional): set it and downloads are 302-redirected there, so the bytes never pass through the relay. For CDNs and public buckets.
- The endpoint accepts `https://host`, `http://host:9000` and `host:9000`. Without a scheme it is treated as https.

**Changing the storage backend does not move existing files.** To move them:

```bash
./nostrel migrate-blobs ./blobs   # old local directory → the currently configured backend
```

Files already present at the destination are skipped, so it is safe to run repeatedly.

### Blossom BUDs

Send a kind 24242 auth event (the `t` tag names the action, `expiration` is required) as `Authorization: Nostr <base64>`.

| BUD | Path | Description |
|---|---|---|
| BUD-01 | `GET /<sha256>[.ext]`, `HEAD /<sha256>` | download and existence check; resolves even when the stored and requested extensions differ |
| BUD-02 | `PUT /upload` | upload; 402 when over quota |
| BUD-02 | `GET /list/<pubkey>` | a user's files |
| BUD-02 | `DELETE /<sha256>` | delete — **uploader or admin only**, refunds the quota |
| BUD-04 | `PUT /mirror` | fetch a URL from another server and store it |
| BUD-05 | `PUT /media` | media upload path (delegates to `/upload`) |
| BUD-06 | `HEAD /upload` | pre-authorize an upload (`X-SHA-256`, `X-Content-Length`) |
| BUD-09 | `PUT /report` | kind 1984 report → lands in the admin queue |

BUD-03 (user server lists, kind 10063) is a client-written event, so the relay simply stores it.

Two of these bypass the engine's default implementation and are handled in `internal/relay`:

- `PUT /report` — the upstream handler reads with `r.Body.Read(nil)`, so the body is always empty and parsing fails.
- `PUT /mirror` — upstream `http.Get`s the user-supplied URL before the policy hook runs, which lets a paying user poke internal networks and cloud metadata (169.254.169.254) through the relay. Here it is fetched with a dialer that refuses loopback, private and link-local addresses, and size and quota are checked before anything is stored.

NIP-96 (NIP-98 authenticated):

| Method | Path | Description |
|---|---|---|
| GET | `/.well-known/nostr/nip96.json` | server configuration |
| POST | `/nip96` | multipart `file` upload → returns NIP-94 tags |
| DELETE | `/nip96/<sha256>` | uploader or admin only |

Both protocols share the same storage and the same ownership index (kind 24242 events). A file uploaded over Blossom can be downloaded from its NIP-96 URL, and the quota is charged once. When several people upload identical content, one copy is kept on disk and only the first uploader is charged.

The admin panel's `Media` section lists every file with its size and uploader and can force-delete them; reported files are shown separately at the top (dismiss or delete).

Example:

```bash
SHA=$(shasum -a 256 pic.png | cut -d' ' -f1)
AUTH=$(nak event -k 24242 -t t=upload -t x=$SHA -t expiration=$(($(date +%s)+300)) --sec $SK | base64)
curl -X PUT https://relay.example.com/upload -H "Authorization: Nostr $AUTH" --data-binary @pic.png
```

## Admin

`PANEL_URL/admin`

- NIP-98: sign with a key listed in `ADMIN_PUBKEYS`. Every request is signed, the URL, method and body hash are all verified, and a 60-second window plus replay blocking applies.
- NIP-07 or NIP-46: the signing key can come from a browser extension or from a remote signer — paste a `bunker://` string or scan the `nostrconnect://` QR. The remote signer signs each request the same way; the key itself never reaches the page or the relay. The QR needs `NIP-46 login relays` set under `Admin`.
- Password: generate a hash with `nostrel hash-password 'password'` and put it in `ADMIN_PASSWORD_HASH`. It is a **bcrypt** hash (`golang.org/x/crypto/bcrypt`, `DefaultCost` = 10) and looks like `$2a$10$…` — the plaintext password never appears in the environment. Verification is `bcrypt.CompareHashAndPassword`; failed attempts are rate-limited to 5 per IP per 15 minutes. The session cookie is HttpOnly, SameSite=Strict, 12 hours.

**Note:** a NIP-98 `u` tag must match `PANEL_URL` plus the request path exactly (to stop signatures obtained on another site from being replayed). If the address you open in the browser differs from `PANEL_URL`, signed login will fail.

### NIP-86 management API

The same admin key gives you every method from a standard relay management client.

| Area | Methods | Behaviour |
|---|---|---|
| pubkey | `allowpubkey` / `banpubkey` / `listallowedpubkeys` / `listbannedpubkeys` | `allowpubkey` grants one period free at current pricing |
| event | `banevent` / `allowevent` / `listbannedevents` / `listallowedevents` | banning deletes the stored copy and blocks re-upload |
| moderation | `listeventsneedingmoderation` | unhandled kind 1984 (NIP-56) reports |
| kind | `allowkind` / `disallowkind` / `listallowedkinds` / `listdisallowedkinds` | a non-empty allow list wins over everything else |
| IP | `blockip` / `unblockip` / `listblockedips` | refuses the websocket connection itself |
| meta | `changerelayname` / `changerelaydescription` / `changerelayicon` | reflected in NIP-11 immediately |
| admin | `grantadmin` / `revokeadmin` | add and remove admins at runtime; keys from `ADMIN_PUBKEYS` cannot be revoked |
| misc | `stats` | account, event, storage and payment statistics |

`grantadmin` and `revokeadmin` are handled by us because go-nostr's NIP-86 decoder asserts the parameters to `[]string` unchecked and panics (see `internal/relay/nip86fix.go`). Delete that file once upstream fixes it.

## API

| Method | Path | Description |
|---|---|---|
| GET | `/api/info` | relay info and price list |
| POST | `/api/order` | issue an invoice; one of the three shapes below |
| GET | `/api/invoice/{hash}` | payment status (re-queries the backend, 2-second cooldown) |
| GET | `/api/invoice/{hash}/qr.png` | invoice QR |
| GET | `/api/account/{pubkey}` | subscription state, quota, group membership |
| GET | `/.well-known/nostr.json?name=` | NIP-05 lookup (CORS open) |
| GET | `/api/nip05/domains` | domains on sale and their prices |
| GET | `/api/nip05/check?domain=&name=` | availability and price |
| GET | `/api/nip05/names/{pubkey}` | identifiers held by that pubkey |
| GET | `/api/group/{id}` | group state, quota, member count |
| GET·PUT·DELETE | `/api/group/{id}/members[/{pubkey}]` | member management (**owner NIP-98 signature** or admin) |
| POST | `/webhook/lnbits` | LNbits payment notification (re-verified before use) |
| — | `/api/admin/*` | stats, accounts, groups, NIP-05, payments, settings (admin only) |

`POST /api/order` body:

```jsonc
{"pubkey": "<hex>", "periods": 1, "extra_mb": 0}                    // relay access
{"pubkey": "<hex>", "group_name": "team", "periods": 1}             // new group (use group_id to extend)
{"pubkey": "<hex>", "nip05_domain": "example.com",
 "nip05_name": "bob", "nip05_periods": 1}                           // NIP-05 identifier
```

LNbits invoices register `PANEL_URL/webhook/lnbits` as their webhook automatically. Even if LNbits cannot reach the panel, the poller catches the payment within 30 seconds.

## Panel languages

The web panel ships in Korean and English. It picks up the browser's language on
the first visit and remembers the choice after that; the picker sits at the
bottom of the sidebar (and on the admin login screen).

Translations live in one file, `web/static/i18n.js`. The **Korean source string
is the key**, so Korean needs no dictionary and an untranslated string falls back
to Korean rather than to an empty box. Static markup is translated once by
walking the document; anything JavaScript builds calls `t()`, which also fills
`{0}`, `{1}`, … placeholders.

To add a language, add one object to `dict` in that file, add its code to
`LANGS`, and add an `<option>` to the pickers in `web/index.html` and
`web/admin.html`. Then check nothing was missed:

```bash
node web/i18n_test.mjs   # renders both pages and fails on any untranslated string
```

## Operational notes

- Terminate TLS at a reverse proxy (caddy/nginx) and pass `X-Forwarded-Proto` and `X-Forwarded-For` through.
- To back up, stop the relay and copy all three `nostrel.db*` files, or use `sqlite3 nostrel.db ".backup"`.
- LNbits response fields differ between versions. Check the target instance's `/docs` (Swagger) before integrating — the current code accepts both `payment_request`/`bolt11` and `paid`/`status`.

## Tests

```bash
go test ./...   # billing idempotency, quota boundaries, NIP-98, DM filter permissions, blob paths/quota,
                # NIP-05 reservation/expiry/premium, group personal-first billing, schema migrations
```

End-to-end with [nak](https://github.com/fiatjaf/nak):

```bash
nak event -k 1 -c "hello" --sec $SK ws://localhost:3334        # rejected without payment
nak event -k 1 -c "hi" --pow 8 --sec $SK ws://localhost:3334   # when min_pow is 8
nak req -k 1059 -p $PK --auth --sec $SK ws://localhost:3334    # DMs are for the parties only
nak count -k 1 ws://localhost:3334                             # NIP-45
nak req -k 1 --search "keyword" ws://localhost:3334            # NIP-50
nak sync ws://relay-a ws://relay-b -k 1                        # NIP-77
curl -s -H 'Accept: application/nostr+json' http://localhost:3334/ | jq '.supported_nips, .fees'
```
