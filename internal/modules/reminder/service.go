package reminder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/reminder/internal/store"
	"html"
	"net/url"
	"slices"
	"time"
)

var ErrForbidden = errors.New("reminder administrator required")
var ErrVersion = errors.New("reminder settings changed")
var ErrSender = errors.New("reminder sender unavailable")

type Character struct {
	ID         int64  `json:"id,string"`
	Name       string `json:"name"`
	Ready      bool   `json:"ready"`
	Owner      []byte `json:"-"`
	Generation int64  `json:"-"`
}
type Result struct {
	ID            int64
	State, Reason string
}
type Service struct {
	Pool          *pgxpool.Pool
	Enabled       bool
	Origin        string
	Administrator func(context.Context, string) (bool, error)
	Own           func(context.Context, string) ([]Character, error)
	Main          func(context.Context, string) (Character, error)
	Users         func(context.Context) ([]string, error)
	Pending       func(context.Context, string) ([]string, error)
	Send          func(context.Context, Character, int64, string, string) Result
	LockAccounts  func(context.Context, pgx.Tx, []string) error
}
type Configuration struct {
	store.Settings
	Characters []Character `json:"characters"`
	SenderName string      `json:"sender_name"`
	Status     string      `json:"status"`
}

func (s *Service) admin(ctx context.Context, user string) error {
	if s.Administrator == nil {
		return ErrForbidden
	}
	ok, err := s.Administrator(ctx, user)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s *Service) sender(ctx context.Context, c store.Settings) (Character, error) {
	if c.Account == "" || c.SenderID <= 0 || s.Own == nil || s.Send == nil {
		return Character{}, ErrSender
	}
	if err := s.admin(ctx, c.Account); err != nil {
		return Character{}, ErrSender
	}
	chars, err := s.Own(ctx, c.Account)
	if err != nil {
		return Character{}, err
	}
	for _, ch := range chars {
		if ch.ID == c.SenderID && bytes.Equal(ch.Owner, c.Owner) && ch.Ready {
			return ch, nil
		}
	}
	return Character{}, ErrSender
}
func (s *Service) Configuration(ctx context.Context, user string) (Configuration, error) {
	if err := s.admin(ctx, user); err != nil {
		return Configuration{}, err
	}
	c, err := store.ReadSettings(ctx, s.Pool, false)
	if err != nil {
		return Configuration{}, err
	}
	chars, err := s.Own(ctx, user)
	if err != nil {
		return Configuration{}, err
	}
	if chars == nil {
		chars = []Character{}
	}
	out := Configuration{Settings: c, Characters: chars, Status: "not_configured"}
	if c.SenderID > 0 {
		ch, e := s.sender(ctx, c)
		out.Status = "authorization_required"
		if e == nil {
			out.SenderName = ch.Name
			out.Status = "ready"
		}
	}
	return out, nil
}

type SaveInput struct {
	Enabled  bool  `json:"enabled"`
	SenderID int64 `json:"sender_id,string"`
	Revision int64 `json:"revision,string"`
}

func (s *Service) Save(ctx context.Context, user string, in SaveInput) (Configuration, error) {
	if err := s.admin(ctx, user); err != nil {
		return Configuration{}, err
	}
	chars, err := s.Own(ctx, user)
	if err != nil {
		return Configuration{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Configuration{}, err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return Configuration{}, ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{user}); err != nil {
		return Configuration{}, err
	}
	c, err := store.ReadSettings(ctx, tx, true)
	if err != nil {
		return Configuration{}, err
	}
	if c.Revision != in.Revision {
		return Configuration{}, ErrVersion
	}
	// Any current administrator may pause an existing sender, without taking over its consent.
	if in.Enabled || in.SenderID != c.SenderID {
		found := false
		for _, ch := range chars {
			if ch.ID == in.SenderID && (!in.Enabled || ch.Ready) {
				c.SenderID = ch.ID
				c.Account = user
				c.Owner = ch.Owner
				found = true
				break
			}
		}
		if !found {
			return Configuration{}, ErrSender
		}
	}
	c.Enabled = in.Enabled
	if in.Enabled {
		if _, err = s.sender(ctx, c); err != nil {
			return Configuration{}, err
		}
	}
	if err = store.SaveSettings(ctx, tx, user, c); err != nil {
		return Configuration{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Configuration{}, err
	}
	return s.Configuration(ctx, user)
}
func (s *Service) Deliveries(ctx context.Context, user, before string) ([]store.Delivery, error) {
	if err := s.admin(ctx, user); err != nil {
		return nil, err
	}
	return store.List(ctx, s.Pool, before)
}
func (s *Service) Retry(ctx context.Context, user, id string) error {
	if err := s.admin(ctx, user); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = store.Retry(ctx, tx, id, user); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) MergeAccountTx(ctx context.Context, tx pgx.Tx, source, target string, apply bool) (json.RawMessage, error) {
	n, err := store.Merge(ctx, tx, source, target, apply)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]int{"deliveries": n})
}

func (s *Service) deliver(ctx context.Context, account string) error {
	if !s.Enabled || s.Pending == nil || s.Main == nil || s.Send == nil {
		return nil
	}
	origin, err := url.Parse(s.Origin)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") {
		return errors.New("reminder origin missing")
	}
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	locked, err := store.Lock(ctx, conn, account)
	if err != nil || !locked {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if store.Unlock(c, conn, account) != nil {
			_ = conn.Conn().Close(c)
		}
	}()
	c, err := store.ReadSettings(ctx, conn, false)
	if err != nil || !c.Enabled {
		return err
	}
	sender, err := s.sender(ctx, c)
	if err != nil {
		return err
	}
	main, err := s.Main(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if main.ID <= 0 {
		return nil
	}
	keys, err := s.Pending(ctx, account)
	if err != nil {
		return err
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if s.LockAccounts == nil {
		return ErrForbidden
	}
	if err = s.LockAccounts(ctx, tx, []string{account, c.Account}); err != nil {
		return err
	}
	current, err := store.ReadSettings(ctx, tx, true)
	if err != nil {
		return err
	}
	if !current.Enabled || current.Revision != c.Revision {
		return nil
	}
	due, err := store.Reconcile(ctx, tx, account, keys)
	if err != nil {
		return err
	}
	limited, err := store.RateLimited(ctx, tx, account)
	if err != nil {
		return err
	}
	if len(due) == 0 || limited {
		return tx.Commit(ctx)
	}
	id, err := store.Claim(ctx, tx, account, sender.ID, main.ID, main.Name, len(keys), due)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// No network within the claim transaction. Recheck identity, permission, phase and revision at the send boundary.
	result := Result{State: "failed", Reason: "context_changed"}
	current, err = store.ReadSettings(ctx, conn, false)
	if err == nil && current.Enabled && current.Revision == c.Revision {
		senderNow, e := s.sender(ctx, current)
		mainNow, me := s.Main(ctx, account)
		fresh, pe := s.Pending(ctx, account)
		slices.Sort(fresh)
		fresh = slices.Compact(fresh)
		if e == nil && me == nil && pe == nil && mainNow.ID == main.ID && bytes.Equal(mainNow.Owner, main.Owner) && senderNow.Generation == sender.Generation && slices.Equal(keys, fresh) {
			body := fmt.Sprintf("你有 %d 项待审批事项。<br><a href=\"%s\">打开审批中心</a>", len(keys), html.EscapeString(s.Origin+"/approvals"))
			result = s.Send(ctx, sender, main.ID, "待审批提醒", body)
			if result.State != "sent" && result.State != "failed" {
				result.State = "unknown"
			}
			if result.State == "sent" && result.ID <= 0 {
				result.State = "unknown"
				result.Reason = "confirmation_required"
			}
		}
	}
	finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return store.Finish(finish, conn, id, result.State, result.Reason, result.ID)
}
