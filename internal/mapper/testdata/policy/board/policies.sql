-- name: out Post p
-- The posts anybody may see listed: out by now.
-- param: now time.Time
NOT p.hidden AND p.status <> 'draft'
  AND (p.publish_at IS NULL OR p.publish_at <= :now)

-- name: visible Post p
-- get: GetPost
-- The posts the user may see: out, or theirs, or any for staff.
-- param: now time.Time
NOT p.hidden AND (p.status <> 'draft' AND (p.publish_at IS NULL OR p.publish_at <= :now)
  OR p.author_id = :viewer_id OR :viewer_is_staff)

-- name: shown Comment c
-- get: GetComment
-- The comments not taken down.
c.id > 0
