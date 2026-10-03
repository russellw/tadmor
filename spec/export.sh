#!/usr/bin/env bash
# Copy the spec, the conformance suite, and the shared schema into a
# counterpart repository.
#
#   spec/export.sh ../tadmor-<platform>     (e.g. ../tadmor-python)
#
# Counterparts are separate repositories that carry their own copy of spec/,
# conformance/, and db/migrations/, taken at a known tadmor commit
# (spec/README.md). This replaces all three directories in the destination
# wholesale and records that commit in spec/UPSTREAM, so the copy is never
# hand-edited. It refuses to
# export uncommitted changes, since UPSTREAM could not name them.
#
# conformance/run-local.sh is left out: it drives tadmor itself. Each
# counterpart writes its own equivalent wrapper. db/migrations/embed.go is
# left out too: it is how tadmor's Go binary carries the migrations.
set -euo pipefail

if [ $# -ne 1 ] || [ ! -d "$1" ]; then
	echo "usage: $0 <counterpart repository directory>" >&2
	exit 2
fi
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$(cd "$1" && pwd)"
if [ "$dest" = "$repo_root" ]; then
	echo "refusing to export tadmor onto itself" >&2
	exit 1
fi

cd "$repo_root"
if [ -n "$(git status --porcelain -- spec conformance db/migrations)" ]; then
	echo "spec/, conformance/, or db/migrations/ has uncommitted changes; commit them first" >&2
	exit 1
fi
commit="$(git rev-parse HEAD)"

rm -rf "$dest/spec" "$dest/conformance" "$dest/db/migrations"
mkdir -p "$dest/spec" "$dest/conformance" "$dest/db/migrations"
git archive HEAD spec conformance db/migrations | tar -x -C "$dest"
rm -f "$dest/conformance/run-local.sh" "$dest/spec/export.sh" "$dest/db/migrations/embed.go"

cat >"$dest/spec/UPSTREAM" <<UPSTREAM
Copied from tadmor by spec/export.sh. Do not edit spec/, conformance/, or
db/migrations/ here; change them in tadmor and re-export.

commit: $commit
date:   $(git show -s --format=%cs "$commit")
UPSTREAM

echo "exported spec/, conformance/, and db/migrations/ at tadmor $commit to $dest"
