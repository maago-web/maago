-- name: ParcelByKey :one Parcel
-- param: key maago.UUID
SELECT id, order_id, key, meta, tags, ref FROM parcels WHERE key = :key
