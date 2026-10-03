# tadmor specification

This directory specifies tadmor's observable behavior independently of
its technology stack. It exists so that counterpart implementations,
the same product built on different stacks to compare supply-chain
footprint, ergonomics, and performance, can be built from a shared
definition of "done" and checked against it mechanically.

tadmor itself (Go + Postgres + React) is the **reference implementation**.
Where this spec and tadmor disagree, that is a bug in one of them; decide
which and fix it.

## Documents

| File | What it pins down |
| ---- | ----------------- |
| [`api.md`](api.md) | The HTTP/JSON contract: conventions, authentication, every endpoint, request and response shapes, status codes. |
| [`domain.md`](domain.md) | The business rules behind the API: entities, lifecycles, money arithmetic, posting rules, multi-currency, orders, inventory, banking, year-end, reports. |
| [`../conformance/`](../conformance/) | A black-box test suite that drives any implementation over HTTP and checks it against the two documents above. |

## What is contract and what is free

**Contract** (an implementation must match):

- The HTTP API in `api.md`: paths, methods, JSON field names and types,
  status codes, and the initial data a fresh instance starts with.
- The business rules in `domain.md`, to the extent they are observable
  through the API: amounts, statuses, journal entries, report figures,
  and which operations are refused.
- Exact decimal arithmetic. Money never passes through binary floating
  point anywhere in the pipeline.

**Reference, not contract** (copy it if it suits the stack, replace it if
not):

- The Postgres schema in `db/migrations/`. Much of tadmor's integrity lives
  in the database (generated columns, constraint triggers, views), and a
  counterpart that reuses the schema inherits those rules for free. A
  counterpart that prefers an ORM, a different database, or application-level
  enforcement may do so, provided the observable behavior matches.
- Code structure, package layout, error *message* wording, logging, the
  migration mechanism, the build system, and the deployment shape.

**Out of scope for conformance** (described here only so a counterpart can
offer an equivalent product):

- The user interface. tadmor ships a React SPA; its screens are listed in
  `domain.md` §13 as a guide, but nothing checks them. The JSON API is
  mandatory even for a counterpart whose UI is server-rendered: it is the
  integration surface and the only thing the conformance suite can test
  across stacks.
- PDF layout. The PDF endpoints must return a valid PDF with the specified
  headers. What goes on the page is described, but not checked byte for
  byte.
- Actual email delivery. The suite runs with email sending disabled (see
  `api.md` §5.11).
- Operational concerns: the deployment model, TLS termination, backups,
  and the out-of-band bootstrap of the first administrator.

## Conformance

The suite in `conformance/` is a single stdlib-only Go program. It runs
against **a freshly initialized instance**, meaning the schema and seed
data are present, exactly one administrator login exists, and there is
nothing else. Several rules involve global state (fiscal-year ordering,
non-overlapping periods, the frozen base currency), so a used database
gives meaningless results.

```sh
go run ./conformance -base-url http://127.0.0.1:8090 \
    -email admin@example.com -password 'the-password'
```

In tadmor, `make conformance` does the whole run: it creates a throwaway
database, starts the server, bootstraps the admin, runs the suite, and
tears everything down. A counterpart should provide its own equivalent
wrapper. See [`conformance/README.md`](../conformance/README.md).

## Freezing and versioning

The spec describes tadmor as of the commit that last touched this
directory. Counterparts target a specific spec commit. If tadmor's
behavior changes, update the spec and the suite in the same commit, so
the three never drift apart.
