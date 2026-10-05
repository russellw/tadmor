#!/usr/bin/env python3
"""Write tadmor's dependencies.json: the manifest of every third-party
package its build resolves, with the publishing identities behind each
(docs/counterpart-metrics.md, "The dependency manifest").

    tools/dependencies.py [REPO] [--test-dir e2e] [--offline] [--check] [--output PATH]

tadmor's ecosystems are Go modules (vendor/modules.txt) and pnpm
(pnpm-lock.yaml v9), so this reads those. Each counterpart writes its own
manifest with its own tooling; tools/measure.py reads only manifests.

Identity lookups read public registry metadata (no package code is
fetched) and are cached across runs; --offline uses the cache only.
--check reports, without writing, whether the committed manifest is current.
Standard library only.
"""

import argparse
import concurrent.futures
import json
import os
import subprocess
import sys
import urllib.parse
import urllib.request
from pathlib import Path

FORMAT = "tadmor-dependencies/1"

# Where the measuring machine builds; optional npm packages for other
# platforms (fsevents, other esbuild binaries) are never installed there.
PLATFORM_OS, PLATFORM_CPU = "linux", "x64"

CACHE_DIR = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache")) / "tadmor-metrics"
CACHE = CACHE_DIR / "npm-metadata.json"


def git_files(repo):
    out = subprocess.run(["git", "-C", repo, "ls-files"], check=True, capture_output=True, text=True).stdout
    return [f for f in out.splitlines() if f]


def count_lines(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            return sum(1 for line in fh if line.strip())
    except OSError:
        return 0


# ---------------------------------------------------------------------------
# Go modules
# ---------------------------------------------------------------------------

def go_modules(repo):
    """Modules compiled into the build, from vendor/modules.txt (those with at
    least one package listed)."""
    path = Path(repo, "vendor", "modules.txt")
    if not path.exists():
        return []
    mods, current, used = [], None, False
    for line in path.read_text().splitlines():
        if line.startswith("# ") and not line.startswith("## "):
            if current and used:
                mods.append(current)
            parts = line[2:].split()
            current, used = (parts[0], parts[1] if len(parts) > 1 else ""), False
        elif line and not line.startswith("#"):
            used = True
    if current and used:
        mods.append(current)
    return mods


def go_owner(module):
    """The publishing identity behind a Go module: its repository owner."""
    if module.startswith("golang.org/x/"):
        return "go-project (golang.org/x)"
    parts = module.split("/")
    if parts[0] in ("github.com", "gitlab.com", "bitbucket.org", "codeberg.org") and len(parts) > 1:
        return f"{parts[0]}/{parts[1]}"
    return parts[0]


# ---------------------------------------------------------------------------
# pnpm lockfile (v9)
# ---------------------------------------------------------------------------

def unquote(s):
    s = s.strip()
    if len(s) >= 2 and s[0] == s[-1] and s[0] in "'\"":
        return s[1:-1]
    return s


def parse_pnpm_lock(path):
    """Return (importer deps by kind, package metadata, snapshot deps)."""
    importers = {"dependencies": set(), "devDependencies": set(), "optionalDependencies": set()}
    packages, snapshots = {}, {}
    section = None
    imp_kind = imp_name = None
    key = sub = None
    for raw in Path(path).read_text().splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        text = raw.strip()
        if indent == 0:
            section = text.rstrip(":")
            continue
        if section == "importers":
            if indent == 4:
                imp_kind = text.rstrip(":") if text.rstrip(":") in importers else None
            elif indent == 6 and imp_kind:
                imp_name = unquote(text.rstrip(":"))
            elif indent == 8 and imp_kind and text.startswith("version:"):
                importers[imp_kind].add(f"{imp_name}@{unquote(text.split(':', 1)[1])}")
        elif section == "packages":
            if indent == 2:
                key = unquote(text.rstrip(":"))
                packages[key] = {}
            elif indent == 4 and key is not None and ":" in text:
                k, v = text.split(":", 1)
                if k in ("os", "cpu"):
                    packages[key][k] = [unquote(x) for x in v.strip().strip("[]").split(",") if x.strip()]
        elif section == "snapshots":
            if indent == 2:
                key = unquote(text.rstrip(":"))
                snapshots[key] = []
                sub = None
            elif indent == 4:
                sub = text.rstrip(":") if text.rstrip(":") in ("dependencies", "optionalDependencies") else None
            elif indent == 6 and sub and ":" in text:
                name, ver = _split_quoted(text) if text[0] in "'\"" else text.split(":", 1)
                snapshots[key].append((unquote(name), unquote(ver), sub == "optionalDependencies"))
    return importers, packages, snapshots


def _split_quoted(text):
    q = text[0]
    end = text.index(q, 1)
    return text[: end + 1], text[end + 2 :]


def base_key(snapshot_key):
    """'echarts-for-react@3.0.6(echarts@5.6.0)' -> 'echarts-for-react@3.0.6'."""
    return snapshot_key.split("(", 1)[0]


def split_name_version(key):
    at = key.rindex("@")
    return key[:at], key[at + 1 :]


def platform_ok(meta):
    oss, cpus = meta.get("os"), meta.get("cpu")
    ok_os = not oss or any(o == PLATFORM_OS or (o.startswith("!") and o[1:] != PLATFORM_OS) for o in oss)
    ok_cpu = not cpus or any(c == PLATFORM_CPU or (c.startswith("!") and c[1:] != PLATFORM_CPU) for c in cpus)
    return ok_os and ok_cpu


def closure(roots, packages, snapshots):
    """Package versions reachable from roots, as installed on this platform."""
    seen, stack = set(), list(roots)
    while stack:
        key = stack.pop()
        base = base_key(key)
        if key in seen or not platform_ok(packages.get(base, {})):
            continue
        seen.add(key)
        for name, ver, _optional in snapshots.get(key, []):
            stack.append(f"{name}@{ver}")
    return {base_key(k) for k in seen}


def npm_metadata(keys, offline):
    """Map package@version -> {"maintainers": npm usernames with publish
    rights at that version, "bytes": unpacked size}, from registry metadata.
    Cached across runs; None where unknown (--offline and not cached)."""
    cache = {}
    if CACHE.exists():
        cache = json.loads(CACHE.read_text())
    missing = [k for k in keys if k not in cache]
    if missing and not offline:
        def fetch(key):
            name, ver = split_name_version(key)
            url = f"https://registry.npmjs.org/{urllib.parse.quote(name, safe='@')}/{ver}"
            with urllib.request.urlopen(url, timeout=30) as resp:
                doc = json.load(resp)
            return key, {
                "maintainers": sorted({m["name"] for m in doc.get("maintainers", []) if m.get("name")}),
                "bytes": doc.get("dist", {}).get("unpackedSize"),
            }
        with concurrent.futures.ThreadPoolExecutor(8) as pool:
            for key, names in pool.map(fetch, missing):
                cache[key] = names
        CACHE.parent.mkdir(parents=True, exist_ok=True)
        CACHE.write_text(json.dumps(cache, indent=0, sort_keys=True))
    return {k: cache.get(k) for k in keys}


def npm_package_lines(lock_dir, keys):
    """Non-blank JS lines installed for these packages, if node_modules exists."""
    store = Path(lock_dir, "node_modules", ".pnpm")
    if not store.exists():
        return None
    total = 0
    for key in keys:
        name, ver = split_name_version(key)
        pkg = store / f"{name.replace('/', '+')}@{ver}" / "node_modules" / name
        for root, _dirs, files in os.walk(pkg):
            for f in files:
                if f.endswith((".js", ".mjs", ".cjs")):
                    total += count_lines(os.path.join(root, f))
    return total


# ---------------------------------------------------------------------------
# The manifest
# ---------------------------------------------------------------------------

def package(ecosystem, name, version, category, identities, evidence):
    return {"ecosystem": ecosystem, "name": name, "version": version, "category": category,
            "identities": identities, "evidence": evidence}


def manifest(repo, test_dirs, offline):
    files = git_files(repo)
    packages, sources, toolchains = [], [], set()

    mods = go_modules(repo)
    for mod, ver in mods:
        packages.append(package("go", mod, ver, "runtime", [go_owner(mod)], "module path (repository owner)"))
    if mods:
        toolchains.add("Go project (toolchain)")
        vendored = [Path(repo, f) for f in files if f.startswith("vendor/")]
        sources.append({"label": "Go vendor/ (runtime)", "bytes": sum(p.stat().st_size for p in vendored),
                        "lines": sum(count_lines(p) for p in vendored if p.suffix == ".go")})

    for lock in [f for f in files if f.endswith("pnpm-lock.yaml") and "node_modules/" not in f]:
        lock_dir = str(Path(repo, lock).parent)
        rel_dir = str(Path(lock).parent)
        importers, pkgs, snapshots = parse_pnpm_lock(Path(repo, lock))
        prod = closure(importers["dependencies"] | importers["optionalDependencies"], pkgs, snapshots)
        dev = closure(importers["devDependencies"], pkgs, snapshots) - prod
        is_test = any(rel_dir == d or rel_dir.startswith(d + "/") for d in test_dirs)
        groups = [("test", prod | dev)] if is_test else [("runtime", prod), ("build", dev)]
        toolchains.update({"Node.js project (runtime for tooling)", "pnpm (via corepack)"})
        meta = npm_metadata(sorted(prod | dev), offline)
        for cat, keys in groups:
            size = 0
            for k in sorted(keys):
                name, ver = split_name_version(k)
                m = meta.get(k)
                ids = None if m is None else [f"npm:{n}" for n in m["maintainers"]]
                packages.append(package("npm", name, ver, cat, ids,
                                        f"https://registry.npmjs.org/{urllib.parse.quote(name, safe='@')}/{ver}"))
                if m is None:
                    size = None
                elif size is not None and m["bytes"] is not None:
                    size += m["bytes"]
            sources.append({"label": f"npm {rel_dir} ({cat})", "bytes": size, "lines": npm_package_lines(lock_dir, keys)})

    packages.sort(key=lambda p: (p["category"], p["ecosystem"], p["name"], p["version"]))
    return {"format": FORMAT, "generator": "tools/dependencies.py (tadmor)", "platform": f"{PLATFORM_OS}/{PLATFORM_CPU}",
            "toolchains": sorted(toolchains), "packages": packages, "sources": sources}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("repo", nargs="?", default=".")
    ap.add_argument("--test-dir", action="append", default=None,
                    help="directory whose lockfile is test-only tooling (default: e2e)")
    ap.add_argument("--offline", action="store_true", help="use cached registry metadata only")
    ap.add_argument("--check", action="store_true", help="report whether dependencies.json is current, without writing it")
    ap.add_argument("--output", help="write the manifest here instead of REPO/dependencies.json")
    args = ap.parse_args()
    m = manifest(args.repo, args.test_dir or ["e2e"], args.offline)
    text = json.dumps(m, indent=1) + "\n"
    path = Path(args.output) if args.output else Path(args.repo, "dependencies.json")
    if args.check:
        current = path.exists() and path.read_text() == text
        print(f"{path} is {'current' if current else 'stale; rerun tools/dependencies.py'}")
        sys.exit(0 if current else 1)
    path.write_text(text)
    print(f"wrote {path}: {len(m['packages'])} packages")


if __name__ == "__main__":
    main()
