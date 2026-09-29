#!/bin/sh
# Regenerates testdata/compat/expected.json from Raoh for Java, and copies its message catalogues to
# testdata/compat/java/ so the tests can hold this package's catalogues to them.
# Needs Java 25 and Maven; the Java artifacts are the versions pom.xml names.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
cp_file=$(mktemp)
trap 'rm -f "$cp_file"' EXIT
mvn -q -f "$here/pom.xml" dependency:build-classpath -Dmdep.outputFile="$cp_file"
java -cp "$(cat "$cp_file")" "$here/Generate.java" \
    "$root/testdata/compat/cases.json" "$root/testdata/compat/expected.json"
mkdir -p "$root/testdata/compat/java"
jar=$(tr ':' '\n' < "$cp_file" | grep '/raoh-[0-9][^/]*\.jar$')
for locale in "" _ja; do
    unzip -p "$jar" "net/unit8/raoh/messages$locale.properties" \
        >| "$root/testdata/compat/java/messages$locale.properties"
done
