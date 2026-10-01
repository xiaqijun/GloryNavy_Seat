-- name: SearchStaticTypes :many
SELECT n.type_id,n.name_zh,n.name_en FROM eve_sde_type_names n JOIN eve_sde_active_names a ON a.release_id=n.release_id
WHERE n.type_id>0 AND (n.type_id::text=sqlc.arg(term)::text OR strpos(lower(n.name_zh),lower(sqlc.arg(term)))>0 OR strpos(lower(n.name_en),lower(sqlc.arg(term)))>0)
ORDER BY CASE WHEN n.type_id::text=sqlc.arg(term) THEN 0 ELSE 1 END,n.type_id LIMIT 30;
