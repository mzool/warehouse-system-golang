-- name: CreateCategory :one
INSERT INTO material_categories (name, description, meta)
VALUES ($1, $2, $3)
RETURNING id, name, description, meta, created_at, updated_at;

-- name: GetCategoryByID :one
SELECT id, name, description, meta, created_at, updated_at
FROM material_categories
WHERE id = $1;

-- name: UpdateCategory :one
UPDATE material_categories
SET name = COALESCE($2, name),
    description = COALESCE($3, description),
    meta = COALESCE($4, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, name, description, meta, created_at, updated_at;

-- name: ListCategories :many
SELECT id, name, description, meta, created_at, updated_at
FROM material_categories
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: DeleteCategory :exec
DELETE FROM material_categories
WHERE id = $1;

-- name: CountCategories :one
SELECT COUNT(*) AS count
FROM material_categories;

-- name: GetCategoryByName :one
SELECT id, name, description, meta, created_at, updated_at
FROM material_categories
WHERE name = $1;


-- name: CreateUnit :one
INSERT INTO measure_units (name, abbreviation, convertion_factor, convert_to)
VALUES ($1, $2, $3, $4)
RETURNING id, name, abbreviation, convertion_factor, convert_to, created_at, updated_at;

-- name: GetUnitByID :one
SELECT id, name, abbreviation, convertion_factor, convert_to, created_at, updated_at
FROM measure_units
WHERE id = $1;

-- name: UpdateUnit :one
UPDATE measure_units
SET name = COALESCE($2, name),
    abbreviation = COALESCE($3, abbreviation),
    convertion_factor = $4,
    convert_to = $5,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, name, abbreviation, convertion_factor, convert_to, created_at, updated_at;

-- name: ListUnits :many
SELECT u.id, u.name, u.abbreviation, u.convertion_factor, u.convert_to, un.name as convert_to_name, u.created_at, u.updated_at
FROM measure_units u LEFT JOIN measure_units un ON u.convert_to = un.id
ORDER BY u.created_at DESC
LIMIT $1 OFFSET $2;

-- name: DeleteUnit :exec
DELETE FROM measure_units
WHERE id = $1;

-- name: CountUnits :one
SELECT COUNT(*) AS count
FROM measure_units;

-- name: GetUnitByName :one
SELECT id, name, abbreviation, convertion_factor, convert_to, created_at, updated_at
FROM measure_units
WHERE name = $1;

-- name: GetUnitByAbbreviation :one
SELECT id, name, abbreviation, convertion_factor, convert_to, created_at, updated_at
FROM measure_units
WHERE abbreviation = $1;

-- name: CheckUnitReferences :one
SELECT COUNT(*) AS count
FROM measure_units
WHERE convert_to = $1;

-- name: CheckUnitUsedByMaterials :one
SELECT COUNT(*) AS count
FROM materials
WHERE measure_unit_id = $1;
