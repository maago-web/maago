-- name: PostList :many PostRow
-- The posts out by now, those whose title has q if given.
-- param: q? string
-- policy: out p
SELECT p.id, p.title
FROM posts p
WHERE {
  out(p)
  {q} AND p.title LIKE '%' || :q || '%'
}
ORDER BY p.id
