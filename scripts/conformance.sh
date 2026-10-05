#!/usr/bin/env bash
# Runs the cases of the Raoh Specification on raoh-go and checks the result
# against conformance/conformance.json with the raoh-verify of the commit
# conformance/spec.lock pins. Needs git, jq and Go.
#
# RAOH_SPECIFICATION_DIR names a checkout to use instead of cloning one; it has
# to be at the pinned commit with no changes to what the commit holds.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT="$ROOT/conformance/target"
LOCK="$ROOT/conformance/spec.lock"
REPOSITORY="$(jq -er .repository "$LOCK")"
REVISION="$(jq -er .revision "$LOCK")"

mkdir -p "$OUT"
rm -f "$OUT/runner-result.json" "$OUT/conformance-report.json"

if [[ -n "${RAOH_SPECIFICATION_DIR:-}" ]]; then
    SPEC="$(cd "$RAOH_SPECIFICATION_DIR" && pwd)"
else
    SPEC="$OUT/raoh-specification"
    if [[ ! -d "$SPEC/.git" ]]; then
        git clone --quiet "https://github.com/$REPOSITORY.git" "$SPEC"
    fi
    if ! git -C "$SPEC" cat-file -e "$REVISION^{commit}" 2>/dev/null; then
        git -C "$SPEC" fetch --quiet origin
    fi
    git -C "$SPEC" -c advice.detachedHead=false checkout --quiet --detach "$REVISION"
fi

HEAD="$(git -C "$SPEC" rev-parse HEAD)"
if [[ "$HEAD" != "$REVISION" ]]; then
    echo "ERROR: $SPEC is at $HEAD, but conformance/spec.lock pins $REVISION" >&2
    exit 1
fi
DIRTY="$(git -C "$SPEC" status --porcelain --untracked-files=all -- \
    specification.json spec catalog schema suite cmd internal go.mod go.sum)"
if [[ -n "$DIRTY" ]]; then
    echo "ERROR: $SPEC has changes the pinned revision does not:" >&2
    echo "$DIRTY" >&2
    exit 1
fi

IMPLEMENTATION_REVISION="$(git -C "$ROOT" rev-parse HEAD)"
if [[ -n "$(git -C "$ROOT" status --porcelain --untracked-files=all -- . ':(exclude)conformance/target')" ]]; then
    IMPLEMENTATION_REVISION="$IMPLEMENTATION_REVISION-dirty"
fi
# The version is the release tag on the commit, vX.Y.Z without the v, and devel on any other commit
# or on a commit with changes. RAOH_GO_VERSION names it instead, for the release workflow, which
# runs this before the tag exists.
if [[ -n "${RAOH_GO_VERSION:-}" ]]; then
    IMPLEMENTATION_VERSION="$RAOH_GO_VERSION"
elif [[ "$IMPLEMENTATION_REVISION" != *-dirty ]] &&
    TAG="$(git -C "$ROOT" tag --points-at HEAD | grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' | sort -V | tail -n 1)" &&
    [[ -n "$TAG" ]]; then
    IMPLEMENTATION_VERSION="${TAG#v}"
else
    IMPLEMENTATION_VERSION="devel"
fi

(cd "$SPEC" && go build -o "$OUT/raoh-verify" ./cmd/raoh-verify)
DIGEST="$("$OUT/raoh-verify" manifest "$SPEC")"

(cd "$ROOT/conformance" && go run . \
    --spec "$SPEC" \
    --revision "$REVISION" \
    --manifest-digest "$DIGEST" \
    --implementation-revision "$IMPLEMENTATION_REVISION" \
    --implementation-version "$IMPLEMENTATION_VERSION" \
    --messages "$ROOT/messages" \
    --out "$OUT/runner-result.json")

"$OUT/raoh-verify" verify \
    --spec "$SPEC" \
    --result "$OUT/runner-result.json" \
    --conformance "$ROOT/conformance/conformance.json" \
    -o "$OUT/conformance-report.json"
