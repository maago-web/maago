-- name: ReadableComments :many CommentRow
-- The comments on the posts the user may read.
-- policy: visible p
SELECT c.id, c.text
FROM comments c
WHERE EXISTS (SELECT 1 FROM posts p WHERE p.id = c.post_id AND visible(p))
ORDER BY c.id
