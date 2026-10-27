-- name: ListCategories :many
-- Live categories first, archived ones after, each by name.
SELECT * FROM categorize.categories ORDER BY archived_at IS NOT NULL, lower(name), id;

-- name: GetCategory :one
SELECT * FROM categorize.categories WHERE id = $1;

-- name: InsertCategory :exec
INSERT INTO categorize.categories (id, name, color, budgeted) VALUES ($1, $2, $3, $4);

-- name: UpdateCategory :execrows
UPDATE categorize.categories SET name = $2, color = $3, budgeted = $4, updated_at = now()
WHERE id = $1 AND archived_at IS NULL;

-- name: ArchiveCategory :execrows
UPDATE categorize.categories SET archived_at = $2, updated_at = now()
WHERE id = $1 AND archived_at IS NULL;

-- name: DeleteCategoryRules :execrows
DELETE FROM categorize.rules WHERE category_id = $1;

-- name: ListRules :many
SELECT * FROM categorize.rules ORDER BY pattern_kind, pattern, account_id NULLS FIRST;

-- name: GetRuleByPattern :one
SELECT * FROM categorize.rules
WHERE pattern_kind = $1 AND pattern = $2 AND account_id IS NOT DISTINCT FROM sqlc.narg(account_id)::uuid;

-- name: SaveRule :exec
INSERT INTO categorize.rules (id, pattern_kind, pattern, account_id, category_id, created_at, updated_at, origin)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    pattern_kind = EXCLUDED.pattern_kind,
    pattern = EXCLUDED.pattern,
    account_id = EXCLUDED.account_id,
    category_id = EXCLUDED.category_id,
    updated_at = EXCLUDED.updated_at;

-- name: GetRule :one
SELECT * FROM categorize.rules WHERE id = $1;

-- name: DeleteRule :execrows
DELETE FROM categorize.rules WHERE id = $1;

-- name: ListSuggestions :many
-- What the model said about each merchant, leaving out answers that name a
-- category since archived.
SELECT s.pattern, s.category_id, s.confidence, s.model, s.asked_at
FROM categorize.suggestions s
LEFT JOIN categorize.categories c ON c.id = s.category_id
WHERE s.category_id IS NULL OR c.archived_at IS NULL;

-- name: SaveSuggestion :exec
INSERT INTO categorize.suggestions (pattern, category_id, confidence, model, asked_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (pattern) DO UPDATE SET
    category_id = EXCLUDED.category_id,
    confidence = EXCLUDED.confidence,
    model = EXCLUDED.model,
    asked_at = EXCLUDED.asked_at;
