-- =====================================================
-- BATCH QUERIES
-- =====================================================

-- name: GetLastBatchNumberForMaterial :one
SELECT batch_number
FROM batches
WHERE material_id = $1
  AND batch_number LIKE $1 || '/' || EXTRACT(YEAR FROM CURRENT_TIMESTAMP)::TEXT || '/%'
ORDER BY created_at DESC
LIMIT 1;

-- name: CheckOpeningStockExists :one
SELECT EXISTS(
    SELECT 1
    FROM batches
    WHERE material_id = $1
      AND batch_number = $1 || '/' || EXTRACT(YEAR FROM CURRENT_TIMESTAMP)::TEXT || '/open'
) AS exists;

-- name: CreateBatch :one
INSERT INTO batches (
    material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11, $12
)
RETURNING id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at;

-- name: UpdateBatchQuantity :one
UPDATE batches
SET current_quantity = current_quantity + $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at;

-- name: GetBatchesByWarehouseAndMaterial :many
SELECT id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at
FROM batches
WHERE warehouse_id = $1
  AND material_id = $2
  AND current_quantity > 0
ORDER BY created_at ASC;

-- name: GetBatchesByWarehouseAndMaterialLIFO :many
SELECT id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at
FROM batches
WHERE warehouse_id = $1
  AND material_id = $2
  AND current_quantity > 0
ORDER BY created_at DESC;

-- name: GetBatchByID :one
SELECT id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at
FROM batches
WHERE id = $1;

-- name: GetBatchesByIDs :many
SELECT id, material_id, supplier_id, warehouse_id, movement_id,
    unit_price, batch_number, manufacture_date, expiry_date,
    start_quantity, current_quantity, notes, meta, created_at, updated_at
FROM batches
WHERE id = ANY($1::int[]);

-- =====================================================
-- STOCK MOVEMENT QUERIES
-- =====================================================

-- name: CreateStockMovement :one
INSERT INTO stock_movements (
    material_id, from_warehouse_id, to_warehouse_id,
    quantity, stock_direction, movement_type,
    reference, performed_by, movement_date, notes
) VALUES (
    $1, $2, $3,
    $4, $5, $6,
    $7, $8, $9, $10
)
RETURNING id, material_id, from_warehouse_id, to_warehouse_id,
    quantity, stock_direction, movement_type,
    reference, performed_by, movement_date, notes, created_at, updated_at;

-- name: GetStockMovementByID :one
SELECT id, material_id, from_warehouse_id, to_warehouse_id,
    quantity, stock_direction, movement_type,
    reference, performed_by, movement_date, notes, created_at, updated_at
FROM stock_movements
WHERE id = $1;

-- name: GetStockMovementsByReference :many
SELECT id, material_id, from_warehouse_id, to_warehouse_id,
    quantity, stock_direction, movement_type,
    reference, performed_by, movement_date, notes, created_at, updated_at
FROM stock_movements
WHERE reference = $1
ORDER BY movement_date DESC;

-- name: GetStockMovementHistory :many
SELECT sm.id, sm.material_id, sm.from_warehouse_id, sm.to_warehouse_id,
    sm.quantity, sm.stock_direction, sm.movement_type,
    sm.reference, sm.performed_by, sm.movement_date, sm.notes,
    sm.created_at, sm.updated_at,
    m.name as material_name,
    u.username as performed_by_username
FROM stock_movements sm
LEFT JOIN materials m ON sm.material_id = m.id
LEFT JOIN users u ON sm.performed_by = u.id
WHERE sm.material_id = $1
ORDER BY sm.movement_date DESC
LIMIT $2 OFFSET $3;

-- name: GetWarehouseStockMovements :many
SELECT sm.id, sm.material_id, sm.from_warehouse_id, sm.to_warehouse_id,
    sm.quantity, sm.stock_direction, sm.movement_type,
    sm.reference, sm.performed_by, sm.movement_date, sm.notes,
    sm.created_at, sm.updated_at,
    m.name as material_name,
    u.username as performed_by_username
FROM stock_movements sm
LEFT JOIN materials m ON sm.material_id = m.id
LEFT JOIN users u ON sm.performed_by = u.id
WHERE (sm.from_warehouse_id = $1 OR sm.to_warehouse_id = $1)
ORDER BY sm.movement_date DESC
LIMIT $2 OFFSET $3;

-- =====================================================
-- VALUATION METHOD QUERIES
-- =====================================================

-- name: GetMaterialValuationMethod :one
SELECT COALESCE(m.valuation, w.valuation) as valuation_method
FROM materials m
CROSS JOIN warehouses w
WHERE m.id = $1 AND w.id = $2;

-- =====================================================
-- TRANSACTION-SPECIFIC QUERIES
-- =====================================================

-- name: GetSaleOrderItemsWithBatches :many
SELECT 
    soi.id as order_item_id,
    soi.material_id,
    soi.quantity,
    soi.shipped_quantity,
    sm.id as movement_id,
    sm.from_warehouse_id,
    b.id as batch_id,
    b.batch_number,
    b.current_quantity as batch_quantity
FROM sales_order_items soi
LEFT JOIN stock_movements sm ON sm.reference = 'SO-' || soi.sales_order_id::TEXT
    AND sm.material_id = soi.material_id
    AND sm.movement_type = 'SALE'
LEFT JOIN batches b ON b.movement_id = sm.id
WHERE soi.sales_order_id = $1;

-- name: GetTransferOutMovementDetails :one
SELECT sm.id, sm.material_id, sm.from_warehouse_id, sm.to_warehouse_id,
    sm.quantity, sm.stock_direction, sm.movement_type,
    sm.reference, sm.performed_by, sm.movement_date, sm.notes,
    sm.created_at, sm.updated_at
FROM stock_movements sm
WHERE sm.id = $1
  AND sm.movement_type = 'TRANSFER_OUT';

-- =====================================================
-- STOCK LEVEL QUERIES
-- =====================================================

-- name: GetCurrentStockLevel :one
SELECT 
    COALESCE(SUM(b.current_quantity), 0) as total_quantity,
    COUNT(DISTINCT b.id) as batch_count
FROM batches b
WHERE b.material_id = $1
  AND b.warehouse_id = $2
  AND b.current_quantity > 0;

-- name: GetStockLevelsByWarehouse :many
SELECT 
    w.id as warehouse_id,
    w.name as warehouse_name,
    m.id as material_id,
    m.name as material_name,
    m.code as material_code,
    COALESCE(SUM(b.current_quantity), 0) as total_quantity,
    COUNT(DISTINCT b.id) as batch_count
FROM warehouses w
CROSS JOIN materials m
LEFT JOIN batches b ON b.warehouse_id = w.id AND b.material_id = m.id AND b.current_quantity > 0
WHERE m.is_active = TRUE
GROUP BY w.id, w.name, m.id, m.name, m.code
HAVING COALESCE(SUM(b.current_quantity), 0) > 0
ORDER BY w.name, m.name;

-- name: GetStockLevelsByMaterial :many
SELECT 
    m.id as material_id,
    m.name as material_name,
    m.code as material_code,
    w.id as warehouse_id,
    w.name as warehouse_name,
    COALESCE(SUM(b.current_quantity), 0) as total_quantity,
    COUNT(DISTINCT b.id) as batch_count,
    MIN(b.unit_price) as min_unit_price,
    MAX(b.unit_price) as max_unit_price,
    AVG(b.unit_price) as avg_unit_price
FROM materials m
LEFT JOIN batches b ON b.material_id = m.id AND b.current_quantity > 0
LEFT JOIN warehouses w ON b.warehouse_id = w.id
WHERE m.id = $1
GROUP BY m.id, m.name, m.code, w.id, w.name
ORDER BY w.name;

-- =====================================================
-- BATCH ALLOCATION QUERIES (for manual selection)
-- =====================================================

-- name: GetAvailableBatchesForMaterial :many
SELECT 
    b.id,
    b.batch_number,
    b.current_quantity,
    b.unit_price,
    b.manufacture_date,
    b.expiry_date,
    w.name as warehouse_name,
    s.name as supplier_name
FROM batches b
LEFT JOIN warehouses w ON b.warehouse_id = w.id
LEFT JOIN suppliers s ON b.supplier_id = s.id
WHERE b.material_id = $1
  AND b.warehouse_id = $2
  AND b.current_quantity > 0
ORDER BY b.created_at ASC;
