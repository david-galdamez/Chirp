-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token, user_id, expires_at, revoked_at)
VALUES ($1, $2, $3, NULL)
RETURNING *;

-- name: GetRefreshToken :one
SELECT user_id, expires_at FROM refresh_tokens WHERE token = $1;

-- name: RevokeToken :one
UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE token = $1 RETURNING *;