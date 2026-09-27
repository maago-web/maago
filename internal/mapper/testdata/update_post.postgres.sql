-- variant 1, args: now, id; slices:
-- title absent, body absent
UPDATE posts SET
  updated_at = $1
WHERE id = $2

-- variant 2, args: body, now, id; slices:
-- title absent, body present
UPDATE posts SET
  body = $1
  updated_at = $2
WHERE id = $3

-- variant 3, args: title, now, id; slices:
-- title present, body absent
UPDATE posts SET
  title = $1,
  updated_at = $2
WHERE id = $3

-- variant 4, args: title, body, now, id; slices:
-- title present, body present
UPDATE posts SET
  title = $1,
  body = $2
  updated_at = $3
WHERE id = $4
