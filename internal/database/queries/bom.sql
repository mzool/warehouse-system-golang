-- name: CreateBillOfMaterial :one
INSERT INTO bills_of_materials (
    finished_material_id, component_material_id, quantity, unit_measure_id, meta,
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, lead_time_days, supplier_id, alternate_component_id, is_active
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
RETURNING 
    id, finished_material_id, component_material_id, quantity, unit_measure_id, meta, 
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, actual_cost, lead_time_days, supplier_id, alternate_component_id, 
    is_active, archived, archived_at, archived_by, created_at, updated_at;

-- name: GetBillOfMaterialByID :one
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.archived_at, b.archived_by, b.created_at, b.updated_at,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.unit_price as component_unit_price,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    s.name as supplier_name,
    alt.name as alternate_component_name,
    alt.code as alternate_component_code
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
LEFT JOIN materials alt ON b.alternate_component_id = alt.id
WHERE b.id = $1;

-- name: GetBillOfMaterialsByFinishedMaterial :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.created_at, b.updated_at,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.sku as component_material_sku,
    cm.unit_price as component_unit_price,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    s.name as supplier_name,
    alt.name as alternate_component_name,
    alt.code as alternate_component_code,
    (b.quantity * (1 + (b.scrap_percentage / 100))) as adjusted_quantity
FROM bills_of_materials b
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
LEFT JOIN materials alt ON b.alternate_component_id = alt.id
WHERE b.finished_material_id = $1 AND b.archived = FALSE
ORDER BY b.priority, b.operation_sequence NULLS LAST, b.id;

-- name: GetBillOfMaterialsByComponent :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.created_at, b.updated_at,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    fm.sku as finished_material_sku,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
WHERE b.component_material_id = $1
ORDER BY b.id;

-- name: UpdateBillOfMaterial :one
UPDATE bills_of_materials
SET
    finished_material_id = COALESCE($2, finished_material_id),
    component_material_id = COALESCE($3, component_material_id),
    quantity = COALESCE($4, quantity),
    unit_measure_id = COALESCE($5, unit_measure_id),
    meta = COALESCE($6, meta),
    scrap_percentage = COALESCE($7, scrap_percentage),
    fixed_quantity = COALESCE($8, fixed_quantity),
    is_optional = COALESCE($9, is_optional),
    priority = COALESCE($10, priority),
    reference_designator = COALESCE($11, reference_designator),
    notes = COALESCE($12, notes),
    effective_date = COALESCE($13, effective_date),
    expiry_date = COALESCE($14, expiry_date),
    version = COALESCE($15, version),
    operation_sequence = COALESCE($16, operation_sequence),
    estimated_cost = COALESCE($17, estimated_cost),
    actual_cost = COALESCE($18, actual_cost),
    lead_time_days = COALESCE($19, lead_time_days),
    supplier_id = COALESCE($20, supplier_id),
    alternate_component_id = COALESCE($21, alternate_component_id),
    is_active = COALESCE($22, is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING 
    id, finished_material_id, component_material_id, quantity, unit_measure_id, meta, 
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, actual_cost, lead_time_days, supplier_id, alternate_component_id, 
    is_active, archived, archived_at, archived_by, created_at, updated_at;

-- name: DeleteBillOfMaterial :exec
DELETE FROM bills_of_materials
WHERE id = $1;

-- name: DeleteBillOfMaterialsByFinishedMaterial :exec
DELETE FROM bills_of_materials
WHERE finished_material_id = $1;

-- name: DeleteBillOfMaterialsByComponent :exec
DELETE FROM bills_of_materials
WHERE component_material_id = $1;

-- name: ListBillsOfMaterials :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.created_at, b.updated_at,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.unit_price as component_unit_price,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    s.name as supplier_name,
    (b.quantity * (1 + (b.scrap_percentage / 100))) as adjusted_quantity
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
WHERE b.archived = FALSE
ORDER BY b.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountBillsOfMaterials :one
SELECT COUNT(*) AS count
FROM bills_of_materials
WHERE archived = FALSE;

-- name: SearchBillsOfMaterials :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.created_at, b.updated_at,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.unit_price as component_unit_price,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    s.name as supplier_name,
    (b.quantity * (1 + (b.scrap_percentage / 100))) as adjusted_quantity
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
WHERE b.archived = FALSE
    AND (sqlc.narg('query')::TEXT IS NULL OR 
     fm.name ILIKE '%' || sqlc.narg('query') || '%' OR 
     fm.code ILIKE '%' || sqlc.narg('query') || '%' OR
     cm.name ILIKE '%' || sqlc.narg('query') || '%' OR
     cm.code ILIKE '%' || sqlc.narg('query') || '%' OR
     b.version ILIKE '%' || sqlc.narg('query') || '%')
ORDER BY b.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSearchBillsOfMaterials :one
SELECT COUNT(*) AS count
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
WHERE b.archived = FALSE
    AND (sqlc.narg('query')::TEXT IS NULL OR 
     fm.name ILIKE '%' || sqlc.narg('query') || '%' OR 
     fm.code ILIKE '%' || sqlc.narg('query') || '%' OR
     cm.name ILIKE '%' || sqlc.narg('query') || '%' OR
     cm.code ILIKE '%' || sqlc.narg('query') || '%' OR
     b.version ILIKE '%' || sqlc.narg('query') || '%');

-- name: CheckBOMExists :one
SELECT EXISTS(
    SELECT 1 FROM bills_of_materials 
    WHERE finished_material_id = $1 
        AND component_material_id = $2 
        AND version = $3
        AND archived = FALSE
) AS exists;

-- name: GetBOMTotalCost :one
SELECT 
    COALESCE(SUM(
        CASE 
            WHEN b.estimated_cost IS NOT NULL THEN b.estimated_cost
            ELSE (b.quantity * (1 + (b.scrap_percentage / 100)) * COALESCE(m.unit_price, 0))
        END
    ), 0) as total_cost
FROM bills_of_materials b
LEFT JOIN materials m ON b.component_material_id = m.id
WHERE b.finished_material_id = $1 
    AND b.is_active = TRUE 
    AND b.archived = FALSE;

-- name: ArchiveBOM :one
UPDATE bills_of_materials
SET archived = TRUE, archived_by = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING 
    id, finished_material_id, component_material_id, quantity, unit_measure_id, meta, 
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, actual_cost, lead_time_days, supplier_id, alternate_component_id, 
    is_active, archived, archived_at, archived_by, created_at, updated_at;

-- name: UnarchiveBOM :one
UPDATE bills_of_materials
SET archived = FALSE, archived_at = NULL, archived_by = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING 
    id, finished_material_id, component_material_id, quantity, unit_measure_id, meta, 
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, actual_cost, lead_time_days, supplier_id, alternate_component_id, 
    is_active, archived, archived_at, archived_by, created_at, updated_at;

-- name: GetBOMsByVersion :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.created_at, b.updated_at,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.unit_price as component_unit_price
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
WHERE b.finished_material_id = $1 AND b.version = $2
ORDER BY b.priority, b.operation_sequence NULLS LAST;

-- name: GetActiveBOMsByFinishedMaterial :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta, 
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator,
    b.notes, b.effective_date, b.expiry_date, b.version, b.operation_sequence,
    b.estimated_cost, b.actual_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id,
    b.is_active, b.archived, b.created_at, b.updated_at,
    cm.name as component_material_name,
    cm.code as component_material_code,
    cm.unit_price as component_unit_price,
    mu.name as unit_name,
    mu.abbreviation as unit_abbreviation,
    s.name as supplier_name,
    alt.name as alternate_component_name,
    (b.quantity * (1 + (b.scrap_percentage / 100))) as adjusted_quantity,
    COALESCE(b.estimated_cost, b.quantity * (1 + (b.scrap_percentage / 100)) * COALESCE(cm.unit_price, 0)) as calculated_cost
FROM bills_of_materials b
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
LEFT JOIN materials alt ON b.alternate_component_id = alt.id
WHERE b.finished_material_id = $1 
    AND b.is_active = TRUE 
    AND b.archived = FALSE
    AND (b.effective_date IS NULL OR b.effective_date <= CURRENT_DATE)
    AND (b.expiry_date IS NULL OR b.expiry_date > CURRENT_DATE)
ORDER BY b.priority, b.operation_sequence NULLS LAST;

-- name: GetBOMCostBreakdown :many
SELECT 
    b.component_material_id,
    cm.name as component_name,
    cm.code as component_code,
    b.quantity,
    b.scrap_percentage,
    (b.quantity * (1 + (b.scrap_percentage / 100))) as adjusted_quantity,
    cm.unit_price as unit_price,
    COALESCE(b.estimated_cost, b.quantity * (1 + (b.scrap_percentage / 100)) * COALESCE(cm.unit_price, 0)) as total_cost,
    mu.abbreviation as unit,
    b.is_optional,
    b.fixed_quantity
FROM bills_of_materials b
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
WHERE b.finished_material_id = $1 
    AND b.is_active = TRUE 
    AND b.archived = FALSE
ORDER BY total_cost DESC;

-- name: GetBOMVersions :many
SELECT DISTINCT version, 
    COUNT(*) as component_count,
    MIN(effective_date) as effective_date,
    MAX(expiry_date) as expiry_date,
    BOOL_AND(is_active) as all_active
FROM bills_of_materials
WHERE finished_material_id = $1
GROUP BY version
ORDER BY version DESC;

-- name: CloneBOMVersion :exec
INSERT INTO bills_of_materials (
    finished_material_id, component_material_id, quantity, unit_measure_id, meta,
    scrap_percentage, fixed_quantity, is_optional, priority, reference_designator, 
    notes, effective_date, expiry_date, version, operation_sequence, 
    estimated_cost, lead_time_days, supplier_id, alternate_component_id, is_active
)
SELECT 
    b.finished_material_id, b.component_material_id, b.quantity, b.unit_measure_id, b.meta,
    b.scrap_percentage, b.fixed_quantity, b.is_optional, b.priority, b.reference_designator, 
    b.notes, $2::DATE as effective_date, NULL as expiry_date, $3 as version, b.operation_sequence, 
    b.estimated_cost, b.lead_time_days, b.supplier_id, b.alternate_component_id, TRUE as is_active
FROM bills_of_materials b
WHERE b.finished_material_id = $1 AND b.version = $4 AND b.archived = FALSE;

-- name: UpdateBOMActualCost :exec
UPDATE bills_of_materials
SET actual_cost = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: BulkUpdateBOMPriority :exec
UPDATE bills_of_materials
SET priority = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY($1::INT[]);

-- name: GetBOMsBySupplier :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity, 
    b.lead_time_days, b.estimated_cost,
    fm.name as finished_material_name,
    fm.code as finished_material_code,
    cm.name as component_material_name,
    cm.code as component_material_code
FROM bills_of_materials b
LEFT JOIN materials fm ON b.finished_material_id = fm.id
LEFT JOIN materials cm ON b.component_material_id = cm.id
WHERE b.supplier_id = $1 AND b.is_active = TRUE AND b.archived = FALSE
ORDER BY b.lead_time_days DESC;

-- name: GetOptionalComponents :many
SELECT 
    b.id, b.finished_material_id, b.component_material_id, b.quantity,
    b.is_optional, b.priority,
    cm.name as component_name,
    cm.code as component_code,
    mu.abbreviation as unit
FROM bills_of_materials b
LEFT JOIN materials cm ON b.component_material_id = cm.id
LEFT JOIN measure_units mu ON b.unit_measure_id = mu.id
WHERE b.finished_material_id = $1 
    AND b.is_optional = TRUE 
    AND b.is_active = TRUE 
    AND b.archived = FALSE
ORDER BY b.priority;
