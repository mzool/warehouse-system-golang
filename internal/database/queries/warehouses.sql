CREATE TABLE IF NOT EXISTS warehouses (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    code VARCHAR(100) UNIQUE NOT NULL,
    location VARCHAR(500),
    description TEXT,
    valuation VALUATION_METHOD NOT NULL DEFAULT 'FIFO', -- e.g., FIFO, LIFO, Weighted Average
    parent_warehouse INT REFERENCES warehouses(id) ON DELETE SET NULL, -- self-referencing for hierarchical warehouses
    capacity DECIMAL(15, 4),
    meta JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
-- name: CreateWarehouse :one
INSERT INTO warehouses (name, code, location, description, valuation, parent_warehouse, capacity, meta)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at;

-- name: GetWarehouseByID :one
SELECT id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at
FROM warehouses
WHERE id = $1;

-- name: GetWarehouseByCode :one
SELECT id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at
FROM warehouses
WHERE code = $1;

-- name: GetWarehouseByName :one
SELECT id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at
FROM warehouses
WHERE name = $1;

-- name: UpdateWarehouse :one
UPDATE warehouses
SET
    name = COALESCE(NULLIF($2, ''), name),
    code = COALESCE(NULLIF($3, ''), code),
    location = COALESCE($4, location),
    description = COALESCE($5, description),
    valuation = COALESCE($6, valuation),
    parent_warehouse = COALESCE($7, parent_warehouse),
    capacity = COALESCE($8, capacity),
    meta = COALESCE($9, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at;

-- name: DeleteWarehouse :exec
DELETE FROM warehouses
WHERE id = $1;

-- name: ListWarehouses :many
SELECT id, name, code, location, description, valuation, parent_warehouse, capacity, meta, created_at, updated_at
FROM warehouses
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;