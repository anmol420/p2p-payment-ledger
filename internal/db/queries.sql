-- name: CreateAccount :one
INSERT INTO accounts (
    owner_name,
    balance
) VALUES (
    $1, $2
)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = $1
LIMIT 1;

-- name: GetAccountForUpdate :one
SELECT * FROM accounts
WHERE id = $1
LIMIT 1
FOR NO KEY UPDATE;

-- name: UpdateAccountBalance :one
UPDATE accounts
SET
    balance = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListAccounts :many
SELECT * FROM accounts
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateTransaction :one
INSERT INTO transactions (
    idempotency_key,
    from_account_id,
    to_account_id,
    amount,
    status
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetTransaction :one
SELECT * FROM transactions
WHERE id = $1
LIMIT 1;

-- name: GetTransactionByIdempotencyKey :one
SELECT * FROM transactions
WHERE idempotency_key = $1
LIMIT 1;

-- name: ListTransactionsByAccount :many
SELECT * FROM transactions
WHERE from_account_id = $1
    OR to_account_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateTransactionStatus :one
UPDATE transactions
SET status = $2
WHERE id = $1
RETURNING *;
