#!/bin/sh
# Builds the documentation site into build/site: the README and docs/ as the
# manual, and the API contract rendered with Redoc under api/.
# Needs mkdocs-material (pip install -r docs/requirements.txt) and the web
# dependencies (npm ci in web/).
set -eu
cd "$(dirname "$0")/.."
src=build/docs-src
rm -rf "$src" build/site
mkdir -p "$src/api" "$src/brand"

repo=https://github.com/adirkuhn/moneyharpy/blob/main
sed -e "s#](docs/countries.md)#](countries.md)#g" \
    -e "s#](LICENSE)#]($repo/LICENSE)#g" \
    -e "s#A running instance renders it at \`/docs/api/\`.#It is published as the [API reference](api/index.html); a running instance renders it at \`/docs/api/\`.#" \
    README.md > "$src/index.md"
cp docs/countries.md "$src/"
cp docs/site/brand.css "$src/"
cp web/public/brand/harpy-mark.svg web/public/brand/harpy-mark-dark.svg "$src/brand/"
cp docs/site/api/index.html docs/site/api/init.js "$src/api/"
# The contract serves from its own instance ("/"); the published copy names
# an example instance instead of the documentation host.
sed '/^servers:$/{n;s#^  - url: /$#  - url: http://localhost:8080\n    description: Your MoneyHarpy instance#;}' api/openapi.yaml > "$src/api/openapi.yaml"
cp web/node_modules/redoc/bundles/redoc.standalone.js "$src/api/"

mkdocs build --quiet
