-- name: PostDetail :one PostWithAuthor
-- A post the user may read, with its author's name.
-- param: id int64
-- column: author? string
-- policy: visible p
SELECT p.id, p.title, u.name AS author
FROM posts p
LEFT JOIN users u ON u.id = p.author_id
WHERE p.id = :id AND visible(p)
