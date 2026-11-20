#!/bin/sh
# Validates every document in /in against both rule sets, one JVM per rule
# set, and prints each SVRL report after a line naming the rules and file.
set -e
for r in CEN-EN16931-UBL PEPPOL-EN16931-UBL; do
  mkdir -p /out/$r
  java net.sf.saxon.Transform -xsl:/validator/rules/$r.xsl -s:/in -o:/out/$r
  for f in /out/$r/*; do
    echo "=== $r $(basename "$f")"
    cat "$f"
    echo
  done
done
echo "=== end"
