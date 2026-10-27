# Pluggy fixtures

Responses from the Pluggy API, in the layout `lastro fixtures` writes:

```
items/<item id>.json           GET /items/{id}
accounts/<item id>.json        GET /accounts?itemId= (the results)
transactions/<account id>.json GET /v2/transactions?accountId= (every page)
investments/<item id>.json     GET /investments?itemId= (every page)
```

**These are synthetic.** They follow Pluggy's schema and the behaviour its docs describe (bank debits negative, card purchases positive, a card bill paid from checking, a Pix between own accounts, pending and future-dated transactions, installments, a purchase in dollars, a date with a time of day) but every value is made up.

To replace them with real responses, anonymized:

```sh
lastro fixtures --out tmp/pluggy     # downloads, prints the report, anonymizes into tmp/pluggy/anonymized
```

Review `tmp/pluggy/anonymized` before copying it here: names, documents, account numbers, ids and free-text words are replaced and amounts are scaled, but investment names and banking vocabulary are kept. `tmp/` is in `.gitignore`; the raw dump never leaves it.
