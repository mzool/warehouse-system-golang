-- name: CreatePurchaseOrder :one
INSERT INTO purchase_orders (order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at;

-- name: GetPurchaseOrderByID :one
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
WHERE id = $1;

-- name: GetPurchaseOrderByOrderNumber :one
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
WHERE order_number = $1;

-- name: UpdatePurchaseOrder :one
UPDATE purchase_orders
SET
    order_number = COALESCE(NULLIF($2, ''), order_number),
    supplier_id = COALESCE($3, supplier_id),
    order_date = COALESCE($4, order_date),
    expected_delivery_date = COALESCE($5, expected_delivery_date),
    status = COALESCE(NULLIF($6, ''), status),
    total_amount = COALESCE($7, total_amount),
    approved_by = COALESCE($8, approved_by),
    meta = COALESCE($9, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at;

-- name: DeletePurchaseOrder :exec
DELETE FROM purchase_orders
WHERE id = $1;

-- name: ListPurchaseOrders :many
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountPurchaseOrders :one
SELECT COUNT(*) AS count
FROM purchase_orders;

-- name: SearchPurchaseOrders :many
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     order_number ILIKE '%' || sqlc.narg('query') || '%' OR 
     status ILIKE '%' || sqlc.narg('query') || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSearchPurchaseOrders :one
SELECT COUNT(*) AS count
FROM purchase_orders
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     order_number ILIKE '%' || sqlc.narg('query') || '%' OR 
     status ILIKE '%' || sqlc.narg('query') || '%');

-- name: ListPurchaseOrdersBySupplier :many
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
WHERE supplier_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPurchaseOrdersByStatus :many
SELECT id, order_number, supplier_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM purchase_orders
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreatePurchaseOrderItem :one
INSERT INTO purchase_order_items (purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity, created_at, updated_at;

-- name: GetPurchaseOrderItemByID :one
SELECT id, purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity, created_at, updated_at
FROM purchase_order_items
WHERE id = $1;

-- name: ListPurchaseOrderItems :many
SELECT id, purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity, created_at, updated_at
FROM purchase_order_items
WHERE purchase_order_id = $1
ORDER BY id;

-- name: UpdatePurchaseOrderItem :one
UPDATE purchase_order_items
SET
    material_id = COALESCE($2, material_id),
    quantity = COALESCE($3, quantity),
    unit_price = COALESCE($4, unit_price),
    total_price = COALESCE($5, total_price),
    received_quantity = COALESCE($6, received_quantity),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity, created_at, updated_at;

-- name: DeletePurchaseOrderItem :exec
DELETE FROM purchase_order_items
WHERE id = $1;

-- name: UpdatePurchaseOrderItemReceivedQuantity :one
UPDATE purchase_order_items
SET
    received_quantity = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, purchase_order_id, material_id, quantity, unit_price, total_price, received_quantity, created_at, updated_at;
