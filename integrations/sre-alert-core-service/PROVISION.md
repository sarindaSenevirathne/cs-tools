# Internal API users

`internal/auth` provides PBKDF2-hashed (10000 iterations, random salt) service-account credentials backed by the `integration_users` PostgreSQL table, plus an `auth.RequireAuth` middleware. `cmd/server` wraps `POST /alertz` with `auth.RequireAuth`, so every wake request needs an `integration_users` credential; provision one here and set it on sre-alert-ingestion-service as `ALERT_CORE_WAKE_USERNAME` / `ALERT_CORE_WAKE_SECRET`, otherwise each wake gets a 401 and alerts are only picked up by the `poll.interval` backstop.

Secrets are never stored in plaintext; only the PBKDF2 hash and salt live in PostgreSQL. Each row also tracks who provisioned it, when it was last modified, when its secret was last rotated, and an optional expiry, so accounts behave closer to real identity records rather than a bare credential pair. There's no admin API or startup seeding, so accounts are managed one at a time with `cmd/user`, run against the same PostgreSQL database and `PG*` env vars the server itself uses.

## cmd/user

```bash
go run ./cmd/user create -username <name> [-secret <value>] [-created-by <who>] [-ttl <duration>] [-clear-expiry]
go run ./cmd/user list [-username <name>]                    # summary table, or one user's full detail
go run ./cmd/user enable -username <name>                    # re-enable a disabled user
go run ./cmd/user disable -username <name>                   # disable a user
```

Load `PG*` from `.env` first (it's gitignored):

```bash
set -a && source .env && set +a
```

## Creating a user

```bash
go run ./cmd/user create -username webhook-integration-user
```

With no `-secret`, one is generated and printed once:

```
created internal user "webhook-integration-user"
secret (shown once, store securely): <generated-secret>
```

Copy it into your secrets manager immediately; it is not recoverable afterwards, only the hash is stored. A stable `id` (UUID) is generated for the account at this point and never changes again, even across rotations.

By default `created_by` is your `$USER` env var (falling back to `"unknown"`); pass `-created-by <who>` to set it explicitly, e.g. for an automated pipeline.

## Setting a secret expiry

```bash
go run ./cmd/user create -username webhook-integration-user -ttl 4320h   # ~180 days
```

`auth.RequireAuth` rejects an expired secret the same way it rejects a wrong one: a generic 401, logged server-side as "secret expired". Omit `-ttl` for a secret that never expires (the default). To remove an expiry you previously set, pass `-clear-expiry` on the next `create`/rotate. `-ttl` and `-clear-expiry` are mutually exclusive.

## Setting a specific secret

```bash
go run ./cmd/user create -username webhook-integration-user -secret 'my-chosen-secret'
```

Prefer reading the value from a file or env var (e.g. `-secret "$(cat secret.txt)"`) over typing it inline, since inline arguments land in your shell history.

## Rotating a secret

Re-running `create` for the same `-username` overwrites that row (upsert), but preserves its identity: `id`, `created_at`, and `created_by` stay exactly as they were (`created_by` only changes if you explicitly pass `-created-by` again), and any existing expiry is preserved unless you pass `-ttl` or `-clear-expiry`. Only `secret_hash`, `salt`, `secret_rotated_at`, and `updated_at` change. To rotate: generate or pick a new secret, re-run the command, then update the caller's stored credential to match. The old secret stops working the moment the row is overwritten, so update the caller first if a brief outage during rotation isn't acceptable.

## Listing users

```bash
go run ./cmd/user list
```

```
USERNAME                       ENABLED  CREATED_BY           EXPIRES_AT
webhook-integration-user       true     thevindu             -
```

For full detail on one user (including `id`, `updated_at`, `secret_rotated_at`, and `last_used_at`):

```bash
go run ./cmd/user list -username webhook-integration-user
```

```
username:           webhook-integration-user
id:                 3f9c1e2a-...
enabled:            true
iterations:         10000
created_at:         2026-09-29T10:00:00Z
created_by:         thevindu
updated_at:         2026-09-29T10:00:00Z
secret_rotated_at:  2026-09-29T10:00:00Z
last_used_at:       -
expires_at:         -
```

`last_used_at` is not updated by `RequireAuth` on `/alertz` and stays `-` (unset); nothing currently writes to it. Only metadata is shown in either view; `secret_hash`/`salt` are never printed.

## Enabling / disabling a user

```bash
go run ./cmd/user disable -username webhook-integration-user
go run ./cmd/user enable -username webhook-integration-user
```

`auth.RequireAuth` rejects any request for a disabled user with a generic 401. Disabling keeps the row (and its hash) intact, so re-enabling doesn't require issuing a new secret. `updated_at` is bumped either way.

## Authenticating

Callers of `POST /alertz` (the route wrapped with `auth.RequireAuth`) can authenticate with either header form:

```bash
# Bearer, base64("username:secret")
TOKEN=$(printf '%s:%s' webhook-integration-user '<secret>' | base64 | tr -d '\n')
curl -X POST https://<host>/alertz -H "Authorization: Bearer $TOKEN"

# Basic, via curl's -u
curl -X POST https://<host>/alertz -u webhook-integration-user:<secret>
```

## Schema

```sql
CREATE TABLE IF NOT EXISTS integration_users (
  username          text PRIMARY KEY,
  id                uuid NOT NULL DEFAULT gen_random_uuid(), -- stable identity id, generated once at creation
  secret_hash       text NOT NULL, -- base64 PBKDF2-SHA256 derived key
  salt              text NOT NULL, -- base64 random salt, unique per user
  iterations        int  NOT NULL, -- PBKDF2 iteration count used for this row
  enabled           boolean NOT NULL DEFAULT true,
  created_at        timestamptz NOT NULL DEFAULT now(), -- account creation time, stable across rotations
  created_by        text NOT NULL DEFAULT '', -- operator who provisioned it
  updated_at        timestamptz NOT NULL DEFAULT now(), -- bumped on every create/rotate/enable/disable
  secret_rotated_at timestamptz NOT NULL DEFAULT to_timestamp(0), -- bumped only when secret_hash/salt actually change
  last_used_at      timestamptz NOT NULL DEFAULT to_timestamp(0), -- not written by RequireAuth; always unset today
  expires_at        timestamptz NOT NULL DEFAULT to_timestamp(0)  -- epoch (or earlier) = never expires
);
```
