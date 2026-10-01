package access

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access/internal/store"
)

type Facts func(context.Context, string) ([]Fact, error)
type Lookup func(context.Context, int64) (Corporation, error)
type Service struct {
	pool   *pgxpool.Pool
	facts  Facts
	lookup Lookup
}

var ErrInvalid = errors.New("invalid access change")
var ErrConflict = errors.New("access role changed")

func New(pool *pgxpool.Pool, facts Facts, lookup Lookup) *Service {
	return &Service{pool, facts, lookup}
}

// IsAdministrator always reads the current site flag; management grants and
// in-game roles cannot stand in for this identity.
func (s *Service) IsAdministrator(ctx context.Context, user string) (bool, error) {
	id, err := uuid(user)
	if err != nil {
		return false, err
	}
	return store.New(s.pool).IsAdministrator(ctx, id)
}

type Role struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Grants  []Grant `json:"grants"`
	Version int64   `json:"version,string"`
}
type Account struct {
	Administrator bool   `json:"administrator"`
	CanManage     bool   `json:"can_manage"`
	CanManageSync bool   `json:"can_manage_sync"`
	Roles         []Role `json:"site_roles"`
	Characters    []Fact `json:"characters"`
}

func uuid(raw string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(raw)
	if err != nil || !id.Valid {
		return id, ErrInvalid
	}
	return id, nil
}
func decodeRoles(rows []store.AccessRole) ([]Role, error) {
	result := make([]Role, 0, len(rows))
	for _, r := range rows {
		g := []Grant{}
		if err := json.Unmarshal(r.Grants, &g); err != nil {
			return nil, err
		}
		result = append(result, Role{ID: r.ID.String(), Name: r.Name, Grants: g, Version: r.Version})
	}
	return result, nil
}
func (s *Service) Account(ctx context.Context, user string) (Account, error) {
	a := Account{Roles: []Role{}, Characters: []Fact{}}
	id, err := uuid(user)
	if err != nil {
		return a, err
	}
	q := store.New(s.pool)
	a.Administrator, err = q.IsAdministrator(ctx, id)
	if err != nil {
		return a, err
	}
	rows, err := q.UserRoles(ctx, id)
	if err != nil {
		return a, err
	}
	a.Roles, err = decodeRoles(rows)
	if err != nil {
		return a, err
	}
	if s.facts != nil {
		a.Characters, err = s.facts(ctx, user)
	}
	a.CanManage = a.Can("access.manage", Corporation{})
	a.CanManageSync = a.Can("eve.sync.manage", Corporation{})
	return a, err
}
func (a Account) Can(permission string, target Corporation) bool {
	grants := []Grant{}
	for _, r := range a.Roles {
		grants = append(grants, r.Grants...)
	}
	return Evaluate(a.Administrator, a.Characters, grants, permission, target, time.Now())
}
func (s *Service) Can(ctx context.Context, user, permission string, target Corporation) (bool, error) {
	if permission == "access.members.read" {
		return s.IsAdministrator(ctx, user)
	}
	if !known(permission) {
		return false, nil
	}
	a, err := s.Account(ctx, user)
	if err != nil {
		return false, err
	}
	return a.Can(permission, target), nil
}
func (s *Service) Roles(ctx context.Context) ([]Role, error) {
	rows, err := store.New(s.pool).ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	return decodeRoles(rows)
}
func (s *Service) SaveRole(ctx context.Context, actor string, r Role) (Role, error) {
	id, err := uuid(r.ID)
	if err != nil {
		return r, err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len([]rune(r.Name)) > 80 || r.Version < 0 {
		return r, ErrInvalid
	}
	if err = ValidateGrants(r.Grants); err != nil {
		return r, errors.Join(ErrInvalid, err)
	}
	grants, err := json.Marshal(r.Grants)
	if err != nil {
		return r, err
	}
	if r.Grants == nil {
		grants = []byte("[]")
	}
	err = s.mutate(ctx, actor, "role.saved", r.ID, func(q *store.Queries) error {
		var row store.AccessRole
		var err error
		if r.Version == 0 {
			row, err = q.CreateRole(ctx, store.CreateRoleParams{ID: id, Name: r.Name, Grants: grants})
		} else {
			row, err = q.UpdateRole(ctx, store.UpdateRoleParams{ID: id, Name: r.Name, Grants: grants, Version: r.Version})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		r.Version = row.Version
		return err
	})
	return r, err
}
func (s *Service) DeleteRole(ctx context.Context, actor, role string, version int64) error {
	id, err := uuid(role)
	if err != nil || version <= 0 {
		return ErrInvalid
	}
	return s.mutate(ctx, actor, "role.deleted", role, func(q *store.Queries) error {
		n, err := q.DeleteRole(ctx, store.DeleteRoleParams{ID: id, Version: version})
		if err == nil && n == 0 {
			return ErrConflict
		}
		return err
	})
}

type AuditEntry struct {
	ID        string    `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) Audit(ctx context.Context, before int64) ([]AuditEntry, string, error) {
	rows, err := store.New(s.pool).ListAudit(ctx, before)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = strconv.FormatInt(rows[49].ID, 10)
	}
	items := make([]AuditEntry, 0, len(rows))
	for _, row := range rows {
		items = append(items, AuditEntry{strconv.FormatInt(row.ID, 10), row.Actor, row.Action, row.Subject, row.CreatedAt.Time})
	}
	return items, next, nil
}

func (s *Service) MemberRoles(ctx context.Context, user string) ([]Role, bool, error) {
	id, err := uuid(user)
	if err != nil {
		return nil, false, err
	}
	q := store.New(s.pool)
	rows, err := q.UserRoles(ctx, id)
	if err != nil {
		return nil, false, err
	}
	roles, err := decodeRoles(rows)
	if err != nil {
		return nil, false, err
	}
	admin, err := q.IsAdministrator(ctx, id)
	return roles, admin, err
}
func (s *Service) Assign(ctx context.Context, actor, user, role string, enabled bool) error {
	uid, err := uuid(user)
	if err != nil {
		return err
	}
	rid, err := uuid(role)
	if err != nil {
		return err
	}
	action := "role.assigned"
	if !enabled {
		action = "role.unassigned"
	}
	return s.mutate(ctx, actor, action, user+"/"+role, func(q *store.Queries) error {
		if enabled {
			return q.AssignRole(ctx, store.AssignRoleParams{UserID: uid, RoleID: rid})
		}
		return q.UnassignRole(ctx, store.UnassignRoleParams{UserID: uid, RoleID: rid})
	})
}

// SetAdministrator is deliberately available only to the local operator CLI.
func (s *Service) SetAdministrator(ctx context.Context, user string, enabled bool) error {
	id, err := uuid(user)
	if err != nil {
		return err
	}
	action := "administrator.added"
	if !enabled {
		action = "administrator.removed"
	}
	return s.mutate(ctx, "local-operator", action, user, func(q *store.Queries) error {
		if enabled {
			return q.SetAdministrator(ctx, id)
		}
		return q.RemoveAdministrator(ctx, id)
	})
}
func (s *Service) mutate(ctx context.Context, actor, action, subject string, fn func(*store.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	q := store.New(tx)
	if err = fn(q); err != nil {
		return err
	}
	if err = q.Audit(ctx, store.AuditParams{Actor: actor, Action: action, Subject: subject}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
