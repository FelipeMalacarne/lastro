# Pluggy fixtures

Responses from the Pluggy API, in the layout `lastrod fixtures` writes:

```
items/<item id>.json           GET /items/{id}
accounts/<item id>.json        GET /accounts?itemId= (the results)
transactions/<account id>.json GET /v2/transactions?accountId= (every page)
investments/<item id>.json     GET /investments?itemId= (every page)
```

Two sets live in `testdata`:

- `pluggy/` (this one): **synthetic**, hand-written scenarios the tests name one by one.
- `meupluggy/`: the anonymized dump of my real MeuPluggy connection (Nubank, 2026-10-27), to check that the mapping and the matcher behave on real data.

**The synthetic set:** They follow Pluggy's schema and the behaviour its docs describe (bank debits negative, card purchases positive, a card bill paid from checking, a Pix between own accounts, pending and future-dated transactions, installments, a purchase in dollars, a date with a time of day) but every value is made up.

To replace them with real responses, anonymized:

```sh
lastrod fixtures --out tmp/pluggy     # downloads, prints the report, anonymizes into tmp/pluggy/anonymized
```

Review `tmp/pluggy/anonymized` before copying it over `../meupluggy`: names, documents, account numbers, ids and free-text words are replaced and amounts are scaled, investment names, codes and prices are scrambled too; banking vocabulary and the bank code at the start of a transfer number are kept. `tmp/` is in `.gitignore`; the raw dump never leaves it.
