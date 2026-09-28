#!/bin/sh
# Regenerates internal/gencase/java_casing.tsv from the Java on the PATH, then casing_table.go.
# Needs Java 25.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
java "$here/Casing.java" >| "$root/internal/gencase/java_casing.tsv"
cd "$root" && go run ./internal/gencase
