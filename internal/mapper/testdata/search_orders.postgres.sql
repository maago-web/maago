-- variant 1, args: limit; slices:
-- status absent, from absent, to absent, sort=latest
-- status absent, from absent, to present, sort=latest
-- status absent, from present, to absent, sort=latest
SELECT id, status, amount, created_at FROM orders
WHERE
  status <> 'deleted'
ORDER BY
  created_at DESC
LIMIT $1

-- variant 2, args: limit; slices:
-- status absent, from absent, to absent, sort=amount
-- status absent, from absent, to present, sort=amount
-- status absent, from present, to absent, sort=amount
SELECT id, status, amount, created_at FROM orders
WHERE
  status <> 'deleted'
ORDER BY
  amount DESC
LIMIT $1

-- variant 3, args: from, to, limit; slices:
-- status absent, from present, to present, sort=latest
SELECT id, status, amount, created_at FROM orders
WHERE
  created_at BETWEEN $1 AND $2
  AND status <> 'deleted'
ORDER BY
  created_at DESC
LIMIT $3

-- variant 4, args: from, to, limit; slices:
-- status absent, from present, to present, sort=amount
SELECT id, status, amount, created_at FROM orders
WHERE
  created_at BETWEEN $1 AND $2
  AND status <> 'deleted'
ORDER BY
  amount DESC
LIMIT $3

-- variant 5, args: status, limit; slices:
-- status present, from absent, to absent, sort=latest
-- status present, from absent, to present, sort=latest
-- status present, from present, to absent, sort=latest
SELECT id, status, amount, created_at FROM orders
WHERE
  status = $1
ORDER BY
  created_at DESC
LIMIT $2

-- variant 6, args: status, limit; slices:
-- status present, from absent, to absent, sort=amount
-- status present, from absent, to present, sort=amount
-- status present, from present, to absent, sort=amount
SELECT id, status, amount, created_at FROM orders
WHERE
  status = $1
ORDER BY
  amount DESC
LIMIT $2

-- variant 7, args: status, from, to, limit; slices:
-- status present, from present, to present, sort=latest
SELECT id, status, amount, created_at FROM orders
WHERE
  status = $1
  AND created_at BETWEEN $2 AND $3
ORDER BY
  created_at DESC
LIMIT $4

-- variant 8, args: status, from, to, limit; slices:
-- status present, from present, to present, sort=amount
SELECT id, status, amount, created_at FROM orders
WHERE
  status = $1
  AND created_at BETWEEN $2 AND $3
ORDER BY
  amount DESC
LIMIT $4
