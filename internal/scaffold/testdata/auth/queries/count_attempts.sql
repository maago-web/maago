-- name: CountAttempts :one AttemptCounts
-- The attempts at action since then, by the email and by the IP.
-- param: action string
-- param: email string
-- param: ip string
-- param: since time.Time
-- column: by_email int64
-- column: by_ip int64
SELECT COUNT(CASE WHEN email = :email THEN 1 END) AS by_email,
       COUNT(CASE WHEN ip = :ip THEN 1 END) AS by_ip
FROM auth_attempt
WHERE action = :action AND created_at > :since
