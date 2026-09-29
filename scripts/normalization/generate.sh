#!/bin/sh
# Regenerates internal/gennorm/java_norm.tsv from the Java on the PATH, then internal/norm/tables.go.
# Needs Java 25.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
java --add-exports java.base/jdk.internal.icu.lang=ALL-UNNAMED \
    --add-exports java.base/jdk.internal.icu.util=ALL-UNNAMED "$here/Norm.java" >| "$root/internal/gennorm/java_norm.tsv"
cd "$root" && go run ./internal/gennorm
