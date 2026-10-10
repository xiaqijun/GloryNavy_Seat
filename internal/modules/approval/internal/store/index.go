package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/platform/reviewqueue"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("approval index unavailable")

type Scope struct {
	Source           string    `json:"source"`
	All              bool      `json:"all,omitempty"`
	Corporation      string    `json:"corporation,omitempty"`
	Accounts         []string  `json:"accounts,omitempty"`
	RestrictAccounts bool      `json:"restrict_accounts,omitempty"`
	Bindings         []Binding `json:"bindings,omitempty"`
	RestrictBindings bool      `json:"restrict_bindings,omitempty"`
}

type Binding struct {
	Account   string `json:"account"`
	Recipient string `json:"recipient"`
}

type Index struct{ Pool *pgxpool.Pool }

func (s Index) ReplaceSource(ctx context.Context, source string, items []reviewqueue.Item) error {
	if s.Pool == nil {
		return ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, "DELETE FROM approval_items WHERE source=$1", source); err != nil {
		return err
	}
	for _, item := range items {
		if item.Source == "" {
			item.Source = source
		}
		if err = upsert(ctx, tx, item); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO approval_projection_runs(source,state,last_completed_at,last_error,item_count)
		VALUES($1,'fresh',now(),'',$2)
		ON CONFLICT(source) DO UPDATE SET state='fresh',last_completed_at=now(),last_error='',item_count=EXCLUDED.item_count`, source, len(items)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upsert(ctx context.Context, db DBTX, item reviewqueue.Item) error {
	if item.ID <= 0 || item.Source == "" || item.Account == "" || item.Time.IsZero() {
		return fmt.Errorf("invalid approval projection %s/%d", item.Source, item.ID)
	}
	var corporation any
	if item.Corporation != "" {
		id, err := strconv.ParseInt(item.Corporation, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("invalid approval corporation %q", item.Corporation)
		}
		corporation = id
	}
	payload := item.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	actionsValue := item.Actions
	if actionsValue == nil {
		actionsValue = []string{}
	}
	actions, _ := json.Marshal(actionsValue)
	processedByValue := item.ProcessedBy
	if processedByValue == nil {
		processedByValue = []string{}
	}
	processedBy, _ := json.Marshal(processedByValue)
	_, err := db.Exec(ctx, `INSERT INTO approval_items
		(source,source_id,source_version,account_id,processed_by,history,corporation_id,kind,bucket,state,status,applicant,recipient,title,reference,amount_minor,unit,occurred_at,payload,actions)
		VALUES($1,$2,$3,$4::uuid,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT(source,source_id) DO UPDATE SET
		 source_version=EXCLUDED.source_version, account_id=EXCLUDED.account_id, processed_by=EXCLUDED.processed_by, history=EXCLUDED.history, corporation_id=EXCLUDED.corporation_id,
		 kind=EXCLUDED.kind, bucket=EXCLUDED.bucket, state=EXCLUDED.state, status=EXCLUDED.status,
		 applicant=EXCLUDED.applicant, recipient=EXCLUDED.recipient, title=EXCLUDED.title, reference=EXCLUDED.reference,
		 amount_minor=EXCLUDED.amount_minor, unit=EXCLUDED.unit, occurred_at=EXCLUDED.occurred_at,
		 payload=EXCLUDED.payload, actions=EXCLUDED.actions, projection_state='fresh', projection_error='', projected_at=now()
		WHERE approval_items.source_version <= EXCLUDED.source_version`,
		item.Source, item.ID, item.Version, item.Account, processedBy, item.History, corporation, item.Kind, bucket(item), item.State, item.Status,
		item.Applicant, item.Recipient, item.Title, item.Reference, item.Amount, item.Unit, item.Time, payload, actions)
	return err
}

func bucket(item reviewqueue.Item) string {
	if item.State == "cancel_requested" {
		return "pending"
	}
	if item.Status == "information" || item.State == "information" {
		return "information"
	}
	if item.Status == "awaiting_acceptance" {
		return "history"
	}
	if item.State == "fulfilled" || item.State == "cancelled" || item.State == "rejected" || item.State == "settled" || item.State == "completed" {
		return "history"
	}
	if item.Status == "mismatch" || item.Status == "multiple_contracts" || item.Status == "issuer_unverified" || item.Status == "contract_claimed" || item.Status == "contract_unavailable" || item.Status == "evidence_unavailable" || item.Status == "snapshot_required" {
		return "exceptions"
	}
	if item.State == "submitted" || item.State == "external" {
		return "pending"
	}
	return "fulfillment"
}

type Result struct {
	Items       []reviewqueue.Item
	Counts      map[string]int64
	Next        reviewqueue.Position
	HasNext     bool
	Unavailable []string
}

func (s Index) List(ctx context.Context, scopes []Scope, actor string, f reviewqueue.Filter, position reviewqueue.Position, limit int) (Result, error) {
	if s.Pool == nil {
		return Result{}, ErrUnavailable
	}
	if len(scopes) == 0 {
		return Result{Items: []reviewqueue.Item{}, Counts: emptyCounts()}, nil
	}
	scopeJSON, err := json.Marshal(scopes)
	if err != nil {
		return Result{}, err
	}
	direction, comparison, order := "ASC", ">", "occurred_at ASC, source ASC, source_id ASC"
	if f.Sort == "time_desc" || (f.Sort == "" && f.View == "history") {
		direction, comparison, order = "DESC", "<", "occurred_at DESC, source DESC, source_id DESC"
	}
	if f.Sort == "id_asc" || f.Sort == "id_desc" {
		if f.Sort == "id_desc" {
			direction, comparison = "DESC", "<"
		}
		order = "source_id " + direction + ", source " + direction
	}
	args := []any{scopeJSON, actor, f.Kind, f.Corporation, f.Account, f.Search, f.Status, f.From, f.Until, f.Mine, f.View}
	where := `EXISTS (SELECT 1 FROM jsonb_array_elements($1::jsonb) scope
		WHERE scope->>'source'=i.source AND (scope->>'all'='true' OR (scope->>'corporation'=coalesce(i.corporation_id::text,'') AND (coalesce(scope->>'restrict_accounts','false') <> 'true' OR scope->'accounts' ? i.account_id::text) AND (coalesce(scope->>'restrict_bindings','false') <> 'true' OR EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(scope->'bindings')='array' THEN scope->'bindings' ELSE '[]'::jsonb END) binding WHERE binding->>'account'=i.account_id::text AND binding->>'recipient'=i.recipient)))))
		AND ($3='' OR i.kind=$3 OR ($3='growth' AND starts_with(i.kind,'growth_')) OR ($3='activity' AND starts_with(i.kind,'activity_')))
		AND ($4='' OR i.corporation_id::text=$4)
		AND ($5='' OR i.account_id::text=$5)
		AND ($6='' OR strpos(lower(concat(i.source_id,' ',i.reference,' ',i.recipient,' ',i.title)),lower($6))>0)
		AND ($7='' OR i.state=$7 OR i.status=$7)
		AND ($8='' OR i.occurred_at >= $8::timestamptz)
		AND ($9='' OR i.occurred_at < $9::timestamptz)
		AND (NOT $10 OR i.processed_by ? $2)`
	// The source contract counts every bucket after the common filters. The
	// selected view is a page-only predicate, so switching tabs never changes
	// the badge counts shown by the approval center.
	pageFilter := `(($11='history' AND i.history) OR ($11<>'history' AND i.bucket=$11 AND i.account_id::text<>$2))`
	if f.ID > 0 {
		pageFilter = fmt.Sprintf("($%d::bigint>0 AND i.source_id=$%d)", len(args)+1, len(args)+1)
		args = append(args, f.ID)
	}
	cursor := "TRUE"
	var cursorTimeArg, cursorIDArg int
	if !position.Time.IsZero() {
		if f.Sort == "id_asc" || f.Sort == "id_desc" {
			cursorIDArg = len(args) + 1
			args = append(args, position.ID)
			cursor = fmt.Sprintf("(i.source_id,i.source) %s ($%d,$%d)", comparison, cursorIDArg, cursorIDArg+1)
			args = append(args, position.Source)
		} else {
			cursorTimeArg = len(args) + 1
			args = append(args, position.Time)
			cursorIDArg = len(args) + 1
			args = append(args, position.Source, position.ID)
			cursor = fmt.Sprintf("(i.occurred_at,i.source,i.source_id) %s ($%d::timestamptz,$%d,$%d)", comparison, cursorTimeArg, cursorIDArg, cursorIDArg+1)
		}
	} else if position.ID > 0 && (f.Sort == "id_asc" || f.Sort == "id_desc") {
		cursorIDArg = len(args) + 1
		args = append(args, position.ID)
		cursor = fmt.Sprintf("i.source_id %s $%d", comparison, cursorIDArg)
	}
	limitArg := len(args) + 1
	args = append(args, limit+1)
	rows, err := s.Pool.Query(ctx, `WITH filtered AS (SELECT i.* FROM approval_items i WHERE `+where+`), counted AS (
		SELECT jsonb_build_object('pending',count(*) FILTER (WHERE bucket='pending' AND account_id::text<>$2), 'information',count(*) FILTER (WHERE bucket='information' AND account_id::text<>$2), 'fulfillment',count(*) FILTER (WHERE bucket='fulfillment' AND account_id::text<>$2), 'exceptions',count(*) FILTER (WHERE bucket='exceptions' AND account_id::text<>$2), 'history',count(*) FILTER (WHERE history)) counts FROM filtered)
		SELECT coalesce(i.source,''),coalesce(i.source_id,0),coalesce(i.source_version,0),coalesce(i.account_id::text,''),coalesce(i.processed_by,'[]'::jsonb),coalesce(i.history,false),coalesce(i.corporation_id::text,''),coalesce(i.kind,''),coalesce(i.state,''),coalesce(i.status,''),coalesce(i.applicant,''),coalesce(i.recipient,''),coalesce(i.title,''),coalesce(i.reference,''),coalesce(i.amount_minor,0),coalesce(i.unit,''),coalesce(i.occurred_at,'epoch'::timestamptz),coalesce(i.payload,'{}'::jsonb),coalesce(i.actions,'[]'::jsonb),(SELECT counts FROM counted)
		FROM (SELECT i.* FROM filtered i WHERE `+pageFilter+` AND `+cursor+` ORDER BY `+order+` LIMIT $`+strconv.Itoa(limitArg)+`) i
		RIGHT JOIN counted ON true`, args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	out := Result{Items: []reviewqueue.Item{}, Counts: emptyCounts()}
	counted := false
	for rows.Next() {
		var item reviewqueue.Item
		var processedBy, actions, counts []byte
		if err = rows.Scan(&item.Source, &item.ID, &item.Version, &item.Account, &processedBy, &item.History, &item.Corporation, &item.Kind, &item.State, &item.Status, &item.Applicant, &item.Recipient, &item.Title, &item.Reference, &item.Amount, &item.Unit, &item.Time, &item.Payload, &actions, &counts); err != nil {
			return Result{}, err
		}
		_ = json.Unmarshal(processedBy, &item.ProcessedBy)
		_ = json.Unmarshal(actions, &item.Actions)
		// A RIGHT JOIN on counted emits one zero-value row for an empty page;
		// consume its counts but never expose it as an approval item.
		if item.Source == "" {
			if !counted {
				_ = json.Unmarshal(counts, &out.Counts)
				counted = true
			}
			continue
		}
		if len(out.Items) < limit {
			out.Items = append(out.Items, item)
		} else {
			out.HasNext = true
		}
		if !counted {
			_ = json.Unmarshal(counts, &out.Counts)
			counted = true
		}
	}
	if err = rows.Err(); err != nil {
		return Result{}, err
	}
	if out.HasNext && len(out.Items) > 0 {
		last := out.Items[len(out.Items)-1]
		out.Next = reviewqueue.Position{Time: last.Time, Source: last.Source, ID: last.ID}
	}
	return out, nil
}

func emptyCounts() map[string]int64 {
	return map[string]int64{"pending": 0, "information": 0, "fulfillment": 0, "exceptions": 0, "history": 0}
}

func (s Index) MarkSource(ctx context.Context, source, state, message string) error {
	if s.Pool == nil {
		return ErrUnavailable
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO approval_projection_runs(source,state,last_started_at,last_error)
		VALUES($1,$2,now(),$3) ON CONFLICT(source) DO UPDATE SET state=EXCLUDED.state,last_started_at=now(),last_error=EXCLUDED.last_error`, source, state, strings.TrimSpace(message))
	return err
}

func (s Index) StaleSources(ctx context.Context, maxAge time.Duration) ([]string, error) {
	if s.Pool == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.Pool.Query(ctx, `SELECT source FROM approval_projection_runs WHERE last_completed_at IS NULL OR last_completed_at < now()-$1::interval OR state='error'`, fmt.Sprintf("%f seconds", maxAge.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

// RecordDualReadDiff persists only comparison diagnostics. It deliberately
// stores the filter and source/version coordinates, never source payloads or
// approval state, so the table can explain a mismatch without becoming a
// second business record store.
func (s Index) RecordDualReadDiff(ctx context.Context, actor, filter, reason, source string, sourceID, indexedVersion, legacyVersion int64) error {
	if s.Pool == nil {
		return ErrUnavailable
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO approval_dual_read_diffs
		(actor_id,filter,reason,source,source_id,indexed_version,legacy_version)
		VALUES($1,$2::jsonb,$3,$4,NULLIF($5,0),NULLIF($6,0),NULLIF($7,0))`,
		actor, filter, reason, source, sourceID, indexedVersion, legacyVersion)
	return err
}
