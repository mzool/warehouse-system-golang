-- name: CreateCustomer :one
INSERT INTO customers (name, contact_name, contact_email, contact_phone, address, meta)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at;

-- name: GetCustomerByID :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
WHERE id = $1;

-- name: GetCustomerByName :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
WHERE name = $1;

-- name: GetCustomerByEmail :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
WHERE contact_email = $1;

-- name: GetCustomerByPhone :one
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
WHERE contact_phone = $1;

-- name: UpdateCustomer :one
UPDATE customers
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

-- name: DeleteCustomer :exec
DELETE FROM customers
WHERE id = $1;

-- name: ListCustomers :many
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountCustomers :one
SELECT COUNT(*) AS count
FROM customers;

-- name: SearchCustomers :many
SELECT id, name, contact_name, contact_email, contact_phone, address, meta, created_at, updated_at
FROM customers
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_email ILIKE '%' || sqlc.narg('query') || '%' OR
     contact_phone ILIKE '%' || sqlc.narg('query') || '%' OR
     address ILIKE '%' || sqlc.narg('query') || '%')
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSearchCustomers :one
SELECT COUNT(*) AS count
FROM customers
WHERE 
    (sqlc.narg('query')::TEXT IS NULL OR 
     name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_name ILIKE '%' || sqlc.narg('query') || '%' OR 
     contact_email ILIKE '%' || sqlc.narg('query') || '%' OR
     contact_phone ILIKE '%' || sqlc.narg('query') || '%' OR
     address ILIKE '%' || sqlc.narg('query') || '%');
