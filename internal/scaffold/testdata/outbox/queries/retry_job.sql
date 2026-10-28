-- name: RetryJob :exec
-- Puts off a job whose attempt failed, unless another worker has taken
-- it since: its attempts are then more.
-- param: id int64
-- param: attempts int64
-- param: at time.Time
-- param: last_error string
UPDATE outbox_job SET run_at = :at, last_error = :last_error
WHERE id = :id AND attempts = :attempts
