-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    $4
)
RETURNING *;

-- name: GetUserByRefreshToken :one
SELECT * FROM refresh_tokens
WHERE token = $1;

-- name: GetUserInfoByRefreshToken :one
SELECT r.token as refresh_token, u.token as user_token
FROM refresh_tokens r
LEFT JOIN users u
    ON user_id = u.id

WHERE r.token = $1;

-- name: RevokeRefreshToken :one
UPDATE refresh_tokens
SET revoked_at = NOW(),
updated_at = NOW()
WHERE token = $1
RETURNING *;
