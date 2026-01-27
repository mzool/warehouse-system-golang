
-- name: CreateMaterial :one
INSERT INTO materials (
    name, description, valuation, type, saleable,
    unit_price, sale_price, category, code, sku, barcode,
    measure_unit_id, weight, volume, density,
    is_toxic, is_flammable, is_fragile,
    image_url, document_url,
    tax_rate, discount_rate,
    is_active, meta
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18,
    $19, $20,
    $21, $22,
    $23, $24
)
RETURNING id, name, description, valuation, type, saleable,
    unit_price, sale_price, category, code, sku, barcode,
    measure_unit_id, weight, volume, density,
    is_toxic, is_flammable, is_fragile,
    image_url, document_url,
    tax_rate, discount_rate,
    is_active, archived, meta, created_at, updated_at;


-- name: BatchCreateMaterials :copyfrom
INSERT INTO materials (
    name, description, type, saleable,
    unit_price, sale_price, category, code, sku, barcode,
    measure_unit_id, weight,
    is_toxic, is_flammable, is_fragile,
    tax_rate, is_active
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9, $10,
    $11, $12,
    $13, $14, $15,
    $16, $17
);

-- name: GetMaterialByID :one
SELECT 
    m.id, m.name, m.description, m.valuation, m.type, m.saleable,
    m.unit_price, m.sale_price, m.category, 
    mc.name as category_name,
    m.code, m.sku, m.barcode,
    m.measure_unit_id, 
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    m.weight, m.volume, m.density,
    m.is_toxic, m.is_flammable, m.is_fragile,
    m.image_url, m.document_url,
    m.tax_rate, m.discount_rate,
    m.is_active, m.archived, m.meta,
    m.created_at, m.updated_at
FROM materials m
LEFT JOIN material_categories mc ON m.category = mc.id
LEFT JOIN measure_units mu ON m.measure_unit_id = mu.id
WHERE m.id = $1 AND m.archived = FALSE;

-- name: GetMaterialByCode :one
SELECT 
    m.id, m.name, m.description, m.type, m.saleable,
    m.unit_price, m.sale_price, m.code, m.sku,
    m.measure_unit_id, m.category,
    m.is_active, m.created_at, m.updated_at
FROM materials m
WHERE m.code = $1 AND m.archived = FALSE;

-- ============================================================================
-- GET MATERIAL BY SKU
-- ============================================================================
-- name: GetMaterialBySKU :one
SELECT 
    m.id, m.name, m.description, m.type, m.saleable,
    m.unit_price, m.sale_price, m.code, m.sku,
    m.measure_unit_id, m.category,
    m.is_active, m.created_at, m.updated_at
FROM materials m
WHERE m.sku = $1 AND m.archived = FALSE;

-- ============================================================================
-- SEARCH MATERIALS (with filters and pagination)
-- ============================================================================
-- name: SearchMaterials :many
SELECT 
    m.id, m.name, m.description, m.valuation, m.type, m.saleable,
    m.unit_price, m.sale_price, m.category,
    mc.name as category_name,
    m.code, m.sku, m.barcode,
    m.measure_unit_id,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    m.weight, m.volume, m.density,
    m.is_toxic, m.is_flammable, m.is_fragile,
    m.image_url, m.document_url,
    m.tax_rate, m.discount_rate,
    m.is_active, m.archived, m.meta,
    m.created_at, m.updated_at
FROM materials m
LEFT JOIN material_categories mc ON m.category = mc.id
LEFT JOIN measure_units mu ON m.measure_unit_id = mu.id
WHERE 
    (sqlc.narg('archived')::BOOLEAN IS NULL OR m.archived = sqlc.narg('archived'))
    AND (sqlc.narg('query')::TEXT IS NULL OR m.name ILIKE '%' || sqlc.narg('query') || '%' OR m.description ILIKE '%' || sqlc.narg('query') || '%' OR m.code ILIKE '%' || sqlc.narg('query') || '%' OR m.sku ILIKE '%' || sqlc.narg('query') || '%')
    AND (sqlc.narg('type_filter')::material_type IS NULL OR m.type = sqlc.narg('type_filter'))
    AND (sqlc.narg('category')::INT IS NULL OR m.category = sqlc.narg('category'))
    AND (sqlc.narg('is_active')::BOOLEAN IS NULL OR m.is_active = sqlc.narg('is_active'))
    AND (sqlc.narg('saleable')::BOOLEAN IS NULL OR m.saleable = sqlc.narg('saleable'))
ORDER BY m.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountMaterials :one
SELECT COUNT(*) AS count
FROM materials m
WHERE 
    (sqlc.narg('archived')::BOOLEAN IS NULL OR m.archived = sqlc.narg('archived'))
    AND (sqlc.narg('query')::TEXT IS NULL OR m.name ILIKE '%' || sqlc.narg('query') || '%' OR m.description ILIKE '%' || sqlc.narg('query') || '%' OR m.code ILIKE '%' || sqlc.narg('query') || '%' OR m.sku ILIKE '%' || sqlc.narg('query') || '%')
    AND (sqlc.narg('type_filter')::material_type IS NULL OR m.type = sqlc.narg('type_filter'))
    AND (sqlc.narg('category')::INT IS NULL OR m.category = sqlc.narg('category'))
    AND (sqlc.narg('is_active')::BOOLEAN IS NULL OR m.is_active = sqlc.narg('is_active'))
    AND (sqlc.narg('saleable')::BOOLEAN IS NULL OR m.saleable = sqlc.narg('saleable'));

-- ============================================================================
-- UPDATE MATERIAL
-- ============================================================================
-- name: UpdateMaterial :one
UPDATE materials
SET 
    name = COALESCE(NULLIF($2, ''), name),
    description = COALESCE($3, description),
    valuation = COALESCE($4, valuation),
    type = COALESCE($5, type),
    saleable = COALESCE($6, saleable),
    unit_price = COALESCE($7, unit_price),
    sale_price = COALESCE($8, sale_price),
    category = COALESCE($9, category),
    code = COALESCE(NULLIF($10, ''), code),
    sku = COALESCE(NULLIF($11, ''), sku),
    barcode = $12,
    measure_unit_id = COALESCE($13, measure_unit_id),
    weight = COALESCE($14, weight),
    volume = COALESCE($15, volume),
    density = COALESCE($16, density),
    is_toxic = COALESCE($17, is_toxic),
    is_flammable = COALESCE($18, is_flammable),
    is_fragile = COALESCE($19, is_fragile),
    image_url = $20,
    document_url = $21,
    tax_rate = COALESCE($22, tax_rate),
    discount_rate = COALESCE($23, discount_rate),
    is_active = COALESCE($24, is_active),
    meta = COALESCE($25, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND archived = FALSE
RETURNING id, name, description, valuation, type, saleable,
    unit_price, sale_price, category, code, sku, barcode,
    measure_unit_id, weight, volume, density,
    is_toxic, is_flammable, is_fragile,
    image_url, document_url,
    tax_rate, discount_rate,
    is_active, archived, meta, created_at, updated_at;


-- name: ArchiveMaterial :exec
UPDATE materials
SET archived = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;


-- name: RestoreMaterial :exec
UPDATE materials
SET archived = FALSE, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;


-- name: DeleteMaterial :exec
DELETE FROM materials
WHERE id = $1;


-- name: CheckDuplicateCode :one
SELECT EXISTS(
    SELECT 1 FROM materials 
    WHERE code = $1 AND archived = FALSE AND id != COALESCE($2, 0)
) AS exists;


-- name: CheckDuplicateSKU :one
SELECT EXISTS(
    SELECT 1 FROM materials 
    WHERE sku = $1 AND archived = FALSE AND id != COALESCE($2, 0)
) AS exists;


-- name: ListActiveMaterials :many
SELECT 
    m.id, m.name, m.code, m.sku, m.type,
    m.unit_price, m.sale_price,
    m.measure_unit_id,
    mu.abbreviation as unit_abbreviation
FROM materials m
LEFT JOIN measure_units mu ON m.measure_unit_id = mu.id
WHERE m.is_active = TRUE AND m.archived = FALSE
ORDER BY m.name ASC
LIMIT $1 OFFSET $2;

-- name: ExportAllMaterials :many
SELECT 
    m.id, m.name, m.description, m.valuation, m.type, m.saleable,
    m.unit_price, m.sale_price, m.category, 
    mc.name as category_name,
    m.code, m.sku, m.barcode,
    m.measure_unit_id, 
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    m.weight, m.volume, m.density,
    m.is_toxic, m.is_flammable, m.is_fragile,
    m.image_url, m.document_url,
    m.tax_rate, m.discount_rate,
    m.is_active, m.archived, m.meta,
    m.created_at, m.updated_at
FROM materials m
LEFT JOIN material_categories mc ON m.category = mc.id
LEFT JOIN measure_units mu ON m.measure_unit_id = mu.id
ORDER BY m.created_at DESC;

-- name: CheckMaterialSaleable :one
SELECT saleable FROM materials WHERE id = $1 AND archived = FALSE;
