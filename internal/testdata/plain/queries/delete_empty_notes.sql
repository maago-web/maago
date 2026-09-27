-- name: DeleteEmptyNotes :exec
DELETE FROM notes WHERE body = ''
