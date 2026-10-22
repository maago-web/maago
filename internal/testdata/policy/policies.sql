-- name: out Post p
-- The posts anybody may see listed: published by now, not hidden.
-- param: now time.Time
NOT p.hidden AND p.status = 'published' AND (p.publish_at IS NULL OR p.publish_at <= :now)

-- name: visible Post p
-- get: GetPost
-- The posts the user may read: those out, their own, their team's
-- published since they joined, and every one for staff.
-- param: now time.Time
NOT p.hidden AND (
  p.status = 'published' AND (p.publish_at IS NULL OR p.publish_at <= :now)
  OR p.author_id = :viewer_id
  OR p.team_id = :viewer_team_id AND p.publish_at >= :viewer_joined_at
  OR :viewer_is_staff)
