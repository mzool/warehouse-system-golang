-- name: CreateSalesOrder :one
INSERT INTO sales_orders (order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at;

-- name: GetSalesOrderByID :one
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
WHERE id = $1;

-- name: GetSalesOrderByOrderNumber :one
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
WHERE order_number = $1;

-- name: UpdateSalesOrder :one
UPDATE sales_orders
SET
    order_number = COALESCE(NULLIF($2, ''), order_number),
    customer_id = COALESCE($3, customer_id),
    order_date = COALESCE($4, order_date),
    expected_delivery_date = COALESCE($5, expected_delivery_date),
    status = COALESCE(NULLIF($6, ''), status),
    total_amount = COALESCE($7, total_amount),
    approved_by = COALESCE($8, approved_by),
    meta = COALESCE($9, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at;

-- name: DeleteSalesOrder :exec
DELETE FROM sales_orders
WHERE id = $1;

-- name: ListSalesOrders :many
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSalesOrders :one
SELECT COUNT(*) AS count
FROM sales_orders;

-- name: SearchSalesOrders :many
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     order_number ILIKE '%' || sqlc.narg('query') || '%' OR 
     status ILIKE '%' || sqlc.narg('query') || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSearchSalesOrders :one
SELECT COUNT(*) AS count
FROM sales_orders
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     order_number ILIKE '%' || sqlc.narg('query') || '%' OR 
     status ILIKE '%' || sqlc.narg('query') || '%');

-- name: ListSalesOrdersByCustomer :many
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
WHERE customer_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListSalesOrdersByStatus :many
SELECT id, order_number, customer_id, order_date, expected_delivery_date, status, total_amount, created_by, approved_by, meta, created_at, updated_at
FROM sales_orders
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateSalesOrderItem :one
INSERT INTO sales_order_items (sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity, created_at, updated_at;

-- name: GetSalesOrderItemByID :one
SELECT id, sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity, created_at, updated_at
FROM sales_order_items
WHERE id = $1;

-- name: ListSalesOrderItems :many
SELECT id, sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity, created_at, updated_at
FROM sales_order_items
WHERE sales_order_id = $1
ORDER BY id;

-- name: UpdateSalesOrderItem :one
UPDATE sales_order_items
SET
    material_id = COALESCE($2, material_id),
    quantity = COALESCE($3, quantity),
    unit_price = COALESCE($4, unit_price),
    total_price = COALESCE($5, total_price),
    shipped_quantity = COALESCE($6, shipped_quantity),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity, created_at, updated_at;

-- name: DeleteSalesOrderItem :exec
DELETE FROM sales_order_items
WHERE id = $1;

-- name: UpdateSalesOrderItemShippedQuantity :one
UPDATE sales_order_items
SET
    shipped_quantity = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, sales_order_id, material_id, quantity, unit_price, total_price, shipped_quantity, created_at, updated_at;
