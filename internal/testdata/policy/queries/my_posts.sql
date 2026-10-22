-- name: MyPosts :many Post
-- The posts of an author that the user may read.
-- param: author_id int64
-- policy: visible p
SELECT * FROM posts p
WHERE p.author_id = :author_id AND visible(p)
ORDER BY p.id
