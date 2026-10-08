package reviewqueue

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func orderFor(f Filter) (direction, comparison string, byID bool) {
	if f.Sort == "id_desc" {
		return "DESC", "<", true
	}
	if f.Sort == "id_asc" {
		return "ASC", ">", true
	}
	if f.Sort == "time_desc" || (f.Sort == "" && f.View == "history") {
		return "DESC", "<", false
	}
	return "ASC", ">", false
}

type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Read consumes a source-owned authorized projection. No table names or user SQL
// are accepted from HTTP. Counts and the bounded page share a statement snapshot.
// Projection columns: id, moment, bucket, history, processed_by, and item (JSON).
func Read(ctx context.Context, db DB, projection string, scope any, actor string, f Filter, p Position, limit int, source string) (Page, error) {
	direction, comparison, byID := orderFor(f)
	cursor := `($12::timestamptz IS NULL OR (moment,$16::text,id) ` + comparison + ` ($12,$13::text,$14::bigint))`
	order := "moment " + direction + ", id " + direction
	if byID {
		// Keep the shared $12 argument typed even though ID ordering does not
		// use a time cursor.
		cursor = `($14::bigint=0 OR (id,$16::text) ` + comparison + ` ($14,$13::text)) AND ($12::timestamptz IS NULL OR TRUE)`
		order = "id " + direction
	}
	sql := `WITH base AS (` + projection + `), filtered AS (
 SELECT * FROM base WHERE
 ($3='' OR item->>'kind'=$3 OR ($3='growth' AND starts_with(item->>'kind','growth_')) OR ($3='activity' AND starts_with(item->>'kind','activity_')))
 AND ($4='' OR item->>'corporation_id'=$4)
 AND ($5='' OR item->>'account_id'=$5)
 AND ($6='' OR strpos(lower(concat(item->>'id',' ',item->>'reference',' ',item->>'recipient',' ',item->>'title')),lower($6))>0)
 AND ($7='' OR item->>'state'=$7 OR item->>'status'=$7)
 AND ($8::timestamptz IS NULL OR moment >= $8)
 AND ($9::timestamptz IS NULL OR moment < $9)
 AND (NOT $10 OR processed_by=$2::text)
 ), counted AS (
 SELECT jsonb_build_object(
 'pending',count(*) FILTER(WHERE bucket='pending' AND item->>'account_id'<>$2::text),
 'information',count(*) FILTER(WHERE bucket='information' AND item->>'account_id'<>$2::text),
 'fulfillment',count(*) FILTER(WHERE bucket='fulfillment' AND item->>'account_id'<>$2::text),
 'exceptions',count(*) FILTER(WHERE bucket='exceptions' AND item->>'account_id'<>$2::text),
 'history',count(*) FILTER(WHERE history)) counts FROM filtered
 ), page AS (
 SELECT item || jsonb_build_object('time',moment,'source',$16::text) item, moment,id FROM filtered
 WHERE ($15::bigint>0 AND id=$15 OR $15=0 AND
 (($11='history' AND history) OR ($11<>'history' AND bucket=$11 AND item->>'account_id'<>$2::text)))
 AND ` + cursor + `
 ORDER BY ` + order + ` LIMIT $17
 ) SELECT coalesce((SELECT jsonb_agg(item ORDER BY ` + order + `) FROM page),'[]'), counts FROM counted`
	var from, until, at any
	if f.From != "" {
		from = f.From
	}
	if f.Until != "" {
		until = f.Until
	}
	if !p.Time.IsZero() {
		at = p.Time
	}
	var raw, counts []byte
	err := db.QueryRow(ctx, sql, scope, actor, f.Kind, f.Corporation, f.Account, f.Search, f.Status, from, until, f.Mine, f.View, at, p.Source, p.ID, f.ID, source, limit).Scan(&raw, &counts)
	out := Page{Items: []Item{}, Counts: map[string]int64{}}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	err = json.Unmarshal(counts, &out.Counts)
	return out, err
}
