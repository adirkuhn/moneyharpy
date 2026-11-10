# Chart of accounts schemes

| File | Scheme | Source | Licence |
|---|---|---|---|
| `nl-rgs-3.8.csv` | Referentie Grootboekschema (RGS) 3.8, MKB edition | [referentiegrootboekschema.nl](https://www.referentiegrootboekschema.nl/definitieve-versie-rgs-38) | Free of licence costs; the RGS initiative adopted [CC0](https://www.referentiegrootboekschema.nl/voorstel-licentievrij-gebruik-rgs) |

Regenerate a file with `go run ./cmd/rgsimport -in "RGS 3.8-def.xlsx" -version 3.8 -out internal/taxonomy/data/nl-rgs-3.8.csv`.
See `docs/countries.md` for adding another country's scheme.
