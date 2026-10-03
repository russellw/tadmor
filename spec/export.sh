#!/usr/bin/env bash
# Copy the spec and the conformance suite into a counterpart repository.
#
#   spec/export.sh ../counterpart-repo
#
# Counterparts are separate repositories that carry their own copy of spec/
# and conformance/, taken at a known tadmor commit (spec/README.md). This
# replaces both directories in the destination wholesale and records that
# commit in spec/UPSTREAM, so the copy is never hand-edited. It refuses to
# export uncommitted changes, since UPSTREAM could not name them.
#
# conformance/run-local.sh is left out: it drives tadmor itself. Each
# counterpart writes its own equivalent wrapper.
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
if [ -n "$(git status --porcelain -- spec conformance)" ]; then
	echo "spec/ or conformance/ has uncommitted changes; commit them first" >&2
	exit 1
fi
commit="$(git rev-parse HEAD)"

rm -rf "$dest/spec" "$dest/conformance"
mkdir -p "$dest/spec" "$dest/conformance"
git archive HEAD spec conformance | tar -x -C "$dest"
rm -f "$dest/conformance/run-local.sh" "$dest/spec/export.sh"

cat >"$dest/spec/UPSTREAM" <<UPSTREAM
Copied from tadmor by spec/export.sh. Do not edit spec/ or conformance/
here; change them in tadmor and re-export.

commit: $commit
date:   $(git show -s --format=%cs "$commit")
UPSTREAM

echo "exported spec/ and conformance/ at tadmor $commit to $dest"
