-- variant 1, args: board; slices:
-- ids empty, flag false
DELETE FROM posts WHERE
  draft
  AND board_id = $1

-- variant 2, args: board; slices:
-- ids empty, flag true
DELETE FROM posts WHERE
  board_id = $1

-- variant 3, args: board; slices: ids
-- ids non-empty, flag false
DELETE FROM posts WHERE
  id IN ($2, $3)
  OR draft
  AND board_id = $1

-- variant 4, args: board; slices: ids
-- ids non-empty, flag true
DELETE FROM posts WHERE
  id IN ($2, $3)
  AND board_id = $1
