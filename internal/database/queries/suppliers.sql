-- name: CreateSupplier :one
INSERT INTO suppliers (name, contact_name, contact_email, contact_phone, address, meta)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at;

-- name: GetSupplierByID :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
WHERE id = $1;

-- name: GetSupplierByName :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
WHERE name = $1;

-- name: GetSupplierByEmail :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
WHERE contact_email = $1;

-- name: GetSupplierByPhone :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
WHERE contact_phone = $1;

-- name: UpdateSupplier :one
UPDATE suppliers
SET
    name = COALESCE(NULLIF($2, ''), name),
    contact_name = COALESCE($3, contact_name),
    contact_email = COALESCE($4, contact_email),
    contact_phone = COALESCE($5, contact_phone),
    address = COALESCE($6, address),
    meta = COALESCE($7, meta),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at;

-- name: DeleteSupplier :exec
DELETE FROM suppliers
WHERE id = $1;

-- name: ListSuppliers :many
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSuppliers :one
SELECT COUNT(*) AS count
FROM suppliers;

-- name: SearchSuppliers :many
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM suppliers
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_email ILIKE '%' || sqlc.narg('query') || '%' OR
     contact_phone ILIKE '%' || sqlc.narg('query') || '%' OR
     address ILIKE '%' || sqlc.narg('query') || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSearchSuppliers :one
SELECT COUNT(*) AS count
FROM suppliers
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_email ILIKE '%' || sqlc.narg('query') || '%' OR
     contact_phone ILIKE '%' || sqlc.narg('query') || '%' OR
     address ILIKE '%' || sqlc.narg('query') || '%');
