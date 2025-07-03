-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password, token)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2,
    $3
)
RETURNING *;

-- name: DeleteUsers :exec
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1;

-- name: StoreUserToken :one
UPDATE users
SET token = $1,
updated_at = NOW()
WHERE email = $2
RETURNING *;