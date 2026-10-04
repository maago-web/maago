-- name: TitleTaken :one Taken
-- Whether a post has the title.
-- param: title string
-- column: taken bool
SELECT EXISTS (SELECT 1 FROM site_post WHERE title = :title) AS taken
