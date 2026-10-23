// Package feedback owns private member bug reports, feature requests and their history.
package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/feedback/internal/store"
)

type Service struct {
	Pool          *pgxpool.Pool
	Administrator func(context.Context, string) (bool, error)
	LockAccounts  func(context.Context, pgx.Tx, []string) error
	Credit        func(context.Context, pgx.Tx, string, string, int64, int64) error
}
type Actor struct{ ID, Name string }
type Ticket struct {
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	RewardMinor *int64     `json:"reward_minor,omitempty"`
	Images      []Image    `json:"images,omitempty"`
	ID          string     `json:"id"`
	AccountID   string     `json:"account_id"`
	AuthorName  string     `json:"author_name"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"`
	Status      string     `json:"status"`
	Revision    int32      `json:"revision"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type Message struct {
	Images     []Image   `json:"images,omitempty"`
	ID         string    `json:"id"`
	AuthorName string    `json:"author_name"`
	Staff      bool      `json:"staff"`
	Body       string    `json:"body"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
type Detail struct {
	Ticket        Ticket    `json:"ticket"`
	Messages      []Message `json:"messages"`
	Administrator bool      `json:"administrator"`
}
type List struct {
	Tickets       []Ticket `json:"tickets"`
	NextBefore    string   `json:"next_before"`
	Administrator bool     `json:"administrator"`
}
type Submission struct {
	Images     []ImageUpload `json:"images,omitempty"`
	RequestKey string        `json:"request_key"`
	Kind       string        `json:"kind"`
	Title      string        `json:"title"`
	Body       string        `json:"body"`
}
type Reply struct {
	Images     []ImageUpload `json:"images,omitempty"`
	RequestKey string        `json:"request_key"`
	Revision   int32         `json:"revision"`
	Body       string        `json:"body"`
	Status     string        `json:"status"`
}
type Fault struct {
	Status        int
	Code, Message string
}

func (e *Fault) Error() string { return e.Message }

var (
	invalid   = &Fault{400, "invalid_feedback", "请检查反馈类型、标题和内容"}
	forbidden = &Fault{403, "feedback_forbidden", "无权处理此反馈"}
	missing   = &Fault{404, "feedback_not_found", "反馈不存在"}
	changed   = &Fault{409, "feedback_changed", "反馈已变化，请刷新后重试"}
	limited   = &Fault{429, "feedback_limit", "今日提交次数已达上限，请明天再试"}
	full      = &Fault{409, "feedback_thread_full", "此反馈的回复已达上限，请提交新的反馈"}
)

func uuid(s string) pgtype.UUID { var v pgtype.UUID; _ = v.Scan(s); return v }
func validKey(s string) bool    { v := uuid(s); return v.Valid && v.Bytes != [16]byte{} }
func kindOK(s string) bool      { return s == "bug" || s == "feature" }
func statusOK(s string) bool {
	return s == "submitted" || s == "in_progress" || s == "accepted" || s == "resolved" || s == "closed"
}
func clean(s string, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(s) <= max
}
func decode[T any](v any) (out T, err error) {
	b, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(b, &out)
	}
	return
}
func get(ctx context.Context, q *store.Queries, id int64) (Ticket, error) {
	v, err := q.GetTicket(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, missing
	}
	if err != nil {
		return Ticket{}, err
	}
	return decode[Ticket](v)
}
func (s *Service) List(ctx context.Context, actor string, all bool, kind, status string, before int64) (List, error) {
	out := List{Tickets: []Ticket{}}
	if (kind != "" && !kindOK(kind)) || (status != "" && !statusOK(status)) || before < 0 {
		return out, invalid
	}
	admin, err := s.Administrator(ctx, actor)
	if err != nil {
		return out, err
	}
	if all && !admin {
		return out, forbidden
	}
	out.Administrator = admin
	rows, err := store.New(s.Pool).ListTickets(ctx, store.ListTicketsParams{AllAccounts: all, Actor: uuid(actor), Kind: kind, Status: status, BeforeID: before})
	if err != nil {
		return out, err
	}
	for i, v := range rows {
		if i == 30 {
			out.NextBefore = out.Tickets[29].ID
			break
		}
		t, err := decode[Ticket](v)
		if err != nil {
			return out, err
		}
		out.Tickets = append(out.Tickets, t)
	}
	return out, nil
}
func (s *Service) Detail(ctx context.Context, actor string, id int64) (Detail, error) {
	out := Detail{Messages: []Message{}}
	admin, err := s.Administrator(ctx, actor)
	if err != nil {
		return out, err
	}
	q := store.New(s.Pool)
	t, err := get(ctx, q, id)
	if err != nil {
		return out, err
	}
	if t.AccountID != actor && !admin {
		return out, missing
	}
	rows, err := q.ListMessages(ctx, id)
	if err != nil {
		return out, err
	}
	for _, v := range rows {
		m, err := decode[Message](v)
		if err != nil {
			return out, err
		}
		out.Messages = append(out.Messages, m)
	}
	out.Ticket = t
	out.Administrator = admin
	err = loadImages(ctx, q, id, &out)
	return out, err
}
func (s *Service) Create(ctx context.Context, actor Actor, in Submission) (Ticket, error) {
	if err := validateImages(in.Images); err != nil {
		return Ticket{}, err
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if !validKey(in.RequestKey) || !kindOK(in.Kind) || in.Title == "" || in.Body == "" || !clean(in.Title, 120) || !clean(in.Body, 6000) {
		return Ticket{}, invalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(context.Background())
	if err = s.LockAccounts(ctx, tx, []string{actor.ID}); err != nil {
		return Ticket{}, err
	}
	q := store.New(tx)
	if err = q.LockWriter(ctx, actor.ID); err != nil {
		return Ticket{}, err
	}
	id, err := q.FindSubmission(ctx, store.FindSubmissionParams{Actor: uuid(actor.ID), RequestKey: uuid(in.RequestKey)})
	if err == nil {
		t, e := get(ctx, q, id)
		if e != nil {
			return t, e
		}
		if t.AccountID != actor.ID || t.Kind != in.Kind || t.Title != in.Title || t.Body != in.Body {
			return Ticket{}, changed
		}
		if err := sameImages(ctx, q, id, pgtype.Int8{}, in.Images); err != nil {
			return Ticket{}, err
		}
		return t, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, err
	}
	n, err := q.RecentSubmissions(ctx, uuid(actor.ID))
	if err != nil {
		return Ticket{}, err
	}
	if n >= 20 {
		return Ticket{}, limited
	}
	id, err = q.InsertTicket(ctx, store.InsertTicketParams{Actor: uuid(actor.ID), AuthorName: actor.Name, RequestKey: uuid(in.RequestKey), Kind: in.Kind, Title: in.Title, Body: in.Body})
	if err != nil {
		return Ticket{}, err
	}
	if err = saveImages(ctx, q, id, pgtype.Int8{}, in.Images); err != nil {
		return Ticket{}, err
	}
	t, err := get(ctx, q, id)
	if err != nil {
		return Ticket{}, err
	}
	return t, tx.Commit(ctx)
}
func (s *Service) Reply(ctx context.Context, actor Actor, id int64, in Reply) (Ticket, error) {
	if err := validateImages(in.Images); err != nil {
		return Ticket{}, err
	}
	in.Body = strings.TrimSpace(in.Body)
	if !validKey(in.RequestKey) || in.Revision < 1 || !clean(in.Body, 2000) || (in.Body == "" && in.Status == "") || (in.Status != "" && !statusOK(in.Status)) {
		return Ticket{}, invalid
	}
	observed, err := s.Detail(ctx, actor.ID, id)
	if err != nil {
		return Ticket{}, err
	}
	if in.Status != "" && !observed.Administrator {
		return Ticket{}, forbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(context.Background())
	if err = s.LockAccounts(ctx, tx, []string{actor.ID, observed.Ticket.AccountID}); err != nil {
		return Ticket{}, err
	}
	q := store.New(tx)
	if err = q.LockWriter(ctx, actor.ID); err != nil {
		return Ticket{}, err
	}
	if _, err = q.LockTicket(ctx, id); err != nil {
		return Ticket{}, err
	}
	t, err := get(ctx, q, id)
	if err != nil {
		return Ticket{}, err
	}
	if t.AccountID != observed.Ticket.AccountID {
		return Ticket{}, changed
	}
	admin, err := s.Administrator(ctx, actor.ID)
	if err != nil {
		return Ticket{}, err
	}
	if t.AccountID != actor.ID && !admin {
		return Ticket{}, missing
	}
	if in.Status != "" && !admin {
		return Ticket{}, forbidden
	}
	old, err := q.FindMessage(ctx, store.FindMessageParams{Actor: uuid(actor.ID), RequestKey: uuid(in.RequestKey)})
	if err == nil {
		if old.TicketID != id || old.Body != in.Body || old.Status != in.Status {
			return Ticket{}, changed
		}
		if err := sameImages(ctx, q, id, pgtype.Int8{Int64: old.ID, Valid: true}, in.Images); err != nil {
			return Ticket{}, err
		}
		return t, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, err
	}
	if t.Revision != in.Revision {
		return Ticket{}, changed
	}
	n, err := q.RecentMessages(ctx, uuid(actor.ID))
	if err != nil {
		return Ticket{}, err
	}
	if n >= 100 && !admin {
		return Ticket{}, limited
	}
	n, err = q.MessageCount(ctx, id)
	if err != nil {
		return Ticket{}, err
	}
	if n >= 100 {
		return Ticket{}, full
	}
	if in.Status == "accepted" {
		if err = s.adoptReward(ctx, tx, actor.ID, id, t); err != nil {
			return Ticket{}, err
		}
	}
	messageID, err := q.InsertMessage(ctx, store.InsertMessageParams{TicketID: id, Actor: uuid(actor.ID), AuthorName: actor.Name, Staff: admin, RequestKey: uuid(in.RequestKey), Body: in.Body, Status: in.Status})
	if err != nil {
		return Ticket{}, err
	}
	if err = saveImages(ctx, q, id, pgtype.Int8{Int64: messageID, Valid: true}, in.Images); err != nil {
		return Ticket{}, err
	}
	n, err = q.UpdateTicket(ctx, store.UpdateTicketParams{ID: id, Revision: in.Revision, Status: in.Status})
	if err != nil {
		return Ticket{}, err
	}
	if n != 1 {
		return Ticket{}, changed
	}
	t, err = get(ctx, q, id)
	if err != nil {
		return Ticket{}, err
	}
	return t, tx.Commit(ctx)
}

// MergeAccountTx is always registered, including when feedback UI is disabled.
func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	q := store.New(tx)
	v, err := q.MergeSummary(ctx, store.MergeSummaryParams{Source: uuid(source), Target: uuid(target)})
	if err != nil {
		return nil, err
	}
	if apply {
		err = q.MergeTickets(ctx, store.MergeTicketsParams{Source: uuid(source), Target: uuid(target)})
	}
	return json.RawMessage(v), err
}
func parseID(s string) (int64, error) {
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil || v < 1 {
		return 0, invalid
	}
	return v, nil
}
