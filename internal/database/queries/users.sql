-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, full_name, role, is_active)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, username, email, full_name, role, is_active, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, username, email, full_name, role, is_active, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT id, username, email, full_name, role, is_active, created_at, updated_at
FROM users
WHERE username = $1;

-- name: GetUserByEmail :one
SELECT id, username, email, full_name, role, is_active, created_at, updated_at
FROM users
WHERE email = $1;

-- name: UpdateUser :one
UPDATE users
SET username = COALESCE($2, username),
    email = COALESCE($3, email),
    full_name = COALESCE($4, full_name),
    role = COALESCE($5, role),
    is_active = COALESCE($6, is_active),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, username, email, full_name, role, is_active, created_at, updated_at;


-- name: ListUsers :many
SELECT id, username, email, full_name, role, is_active, created_at, updated_at
FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;


-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;

-- name: CountUsers :one
SELECT COUNT(*) AS count
FROM users;


-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: DeactivateUser :exec
UPDATE users
SET is_active = FALSE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: ActivateUser :exec
UPDATE users
SET is_active = TRUE,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;