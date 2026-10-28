-- name: ClaimJob :one Job
-- Takes the job due first, for a worker until :until: of two workers at
-- once, only one gets it. On PostgreSQL both may pick the same id; the
-- second waits for the first's row lock, then checks the outer
-- conditions again on the row the first wrote, and gets no row.
-- param: now time.Time
-- param: until time.Time
-- param: max int64
UPDATE outbox_job SET run_at = :until, attempts = attempts + 1
WHERE id = (SELECT id FROM outbox_job WHERE run_at <= :now AND attempts < :max ORDER BY run_at, id LIMIT 1)
  AND run_at <= :now AND attempts < :max
RETURNING *
