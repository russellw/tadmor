The goal of this project is to develop comprehensive business management software.

Technology stack:
Postgres for the database.
Go for the back end.
TypeScript and React for the front end.
Python as necessary for ancillary scripts.

Schema design:
Where a good natural key presents itself, such as the ISO 2 letter country code, it shall be used.
Where a synthetic key is needed, it shall be an auto-incrementing integer. This forgoes the distributed merge advantages of a UUID key, but gains performance and ease of debugging.

Dependencies:
Supply-chain conscious throughout; keep the third-party footprint small, pinned, and reviewable in-repo.
Back end: standard library first; the only permitted third-party modules are github.com/jackc/pgx/v5 and golang.org/x/*. New modules need a conversation first. Pin exact versions, vendor all source (go mod vendor), and build hermetically (GOPROXY=off, -mod=vendor).
Front end: the web/ stack and its supply-chain posture are ratified in docs/frontend-stack.md — read it before adding any frontend dependency or changing the build or vendoring. UI is vendored shadcn source (no UI-kit dependency); never commit node_modules/ (the committed pnpm-lock.yaml is the go.sum analog); new dependencies must clear the cooldown and script-blocking policy described there.

Version control:
Commit directly to the default branch. Do not create feature branches.
