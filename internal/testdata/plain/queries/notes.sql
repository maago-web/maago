-- name: Notes :many Note
-- A bool condition: two texts.
-- param: long bool
SELECT id, body FROM notes
{long} WHERE length(body) > 3
ORDER BY id
