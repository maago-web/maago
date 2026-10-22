-- name: EveryPost :one int64
-- How many posts there are, hidden and drafts too.
-- policy: none posts a count, which shows no post
SELECT count(*) AS n FROM posts
