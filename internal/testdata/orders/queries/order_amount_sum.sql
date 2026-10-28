-- name: OrderAmountSum :one int64
-- The money of every order, in cents. PostgreSQL sums a bigint into a
-- numeric, so the CAST, which both databases take, makes it an int64.
SELECT CAST(COALESCE(SUM(amount), 0) AS BIGINT) FROM orders
