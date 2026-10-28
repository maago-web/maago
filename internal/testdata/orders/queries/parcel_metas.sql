-- name: ParcelMetas :many ParcelMetaRow
-- A new type with a JSON column, one that may be NULL, and a UUID.
SELECT p.key, p.meta, p.tags, o.status
FROM parcels p JOIN orders o ON o.id = p.order_id
ORDER BY p.id
