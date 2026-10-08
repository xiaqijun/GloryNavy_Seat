package loan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/contractamount"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

var (
	ErrInvalid   = errors.New("invalid loan request")
	ErrForbidden = errors.New("loan access denied")
	ErrRule      = errors.New("loan rule is not configured")
	ErrLimit     = errors.New("loan credit limit exceeded")
	ErrCoverage  = errors.New("loan guarantee or collateral coverage is insufficient")
	ErrConflict  = errors.New("loan record changed")
	ErrContract  = errors.New("loan contract evidence is not valid")
)

type Character struct {
	ID   int64
	Name string
}
type Contract struct {
	ID                  int64
	OwnerKind           string
	OwnerID             int64
	Type                string
	Status              string
	Price               string
	Reward              string
	IssuerID            int64
	AssigneeID          int64
	AcceptorID          int64
	ForCorporation      bool
	IssuerCorporationID int64
	Items               json.RawMessage
	ItemsReady          bool
	Completed           string
}
type ContractReader interface {
	Read(context.Context, string, string, int64, int64) (Contract, error)
	FindCash(context.Context, string, time.Time) ([]Contract, error)
	Claim(context.Context, pgx.Tx, int64, string, int64) error
}
type ContractAdapter struct {
	ReadFunc     func(context.Context, string, string, int64, int64) (Contract, error)
	FindCashFunc func(context.Context, string, time.Time) ([]Contract, error)
	ClaimFunc    func(context.Context, pgx.Tx, int64, string, int64) error
}

func (a ContractAdapter) Read(ctx context.Context, actor, kind string, owner, id int64) (Contract, error) {
	if a.ReadFunc == nil {
		return Contract{}, ErrContract
	}
	return a.ReadFunc(ctx, actor, kind, owner, id)
}
func (a ContractAdapter) FindCash(ctx context.Context, actor string, since time.Time) ([]Contract, error) {
	if a.FindCashFunc == nil {
		return nil, ErrContract
	}
	return a.FindCashFunc(ctx, actor, since)
}
func (a ContractAdapter) Claim(ctx context.Context, tx pgx.Tx, id int64, module string, ref int64) error {
	if a.ClaimFunc == nil {
		return ErrContract
	}
	return a.ClaimFunc(ctx, tx, id, module, ref)
}

type Service struct {
	Pool                *pgxpool.Pool
	Characters          func(context.Context, string) ([]Character, error)
	CanManagePool       func(context.Context, string, string, int64) (bool, error)
	IsAdministrator     func(context.Context, string) (bool, error)
	Contracts           ContractReader
	LockAccounts        func(context.Context, pgx.Tx, []string) error
	ReadContractTx      func(context.Context, pgx.Tx, string, string, int64, int64) (Contract, error)
	AccountForCharacter func(context.Context, int64) (string, error)
	CreditEvidence      func(context.Context, string) (CreditEvidence, error)
}

type ApplicationInput struct {
	PoolID              int64  `json:"pool_id"`
	BorrowerCharacterID int64  `json:"borrower_character_id"`
	PrincipalMinor      int64  `json:"principal_minor"`
	InterestMinor       int64  `json:"interest_minor"`
	InstallmentCount    int    `json:"installment_count"`
	IntervalDays        int    `json:"interval_days"`
	FirstDueAt          string `json:"first_due_at"`
}
type ReviewInput struct {
	State   string `json:"state"`
	Version int64  `json:"version"`
	Note    string `json:"note"`
}
type PaymentInput struct {
	Kind                 string `json:"kind"`
	Version              int64  `json:"version"`
	ContractKind         string `json:"contract_kind"`
	ContractOwnerID      int64  `json:"contract_owner_id"`
	ContractID           int64  `json:"contract_id"`
	RecipientCharacterID int64  `json:"recipient_character_id"`
	ExpectedMinor        int64  `json:"expected_minor"`
}
type GuaranteeInput struct {
	GuarantorAccountID   string `json:"guarantor_account_id"`
	GuarantorCharacterID int64  `json:"guarantor_character_id"`
	AmountMinor          int64  `json:"amount_minor"`
	Note                 string `json:"note"`
}
type CollateralInput struct {
	OwnerAccountID  string          `json:"owner_account_id"`
	ContractKind    string          `json:"contract_kind"`
	ContractOwnerID int64           `json:"contract_owner_id"`
	ContractID      int64           `json:"contract_id"`
	Items           json.RawMessage `json:"items"`
	ValuationMinor  int64           `json:"valuation_minor"`
	HaircutBPS      int             `json:"haircut_bps"`
	CoveredMinor    int64           `json:"covered_minor"`
}
type DecisionInput struct {
	Version int64  `json:"version"`
	State   string `json:"state"`
}

func publicID(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102"), strings.ToUpper(fmt.Sprintf("%x", b)))
}
func (s *Service) db() store.DBTX { return s.Pool }

func (s *Service) Pools(ctx context.Context, user string, admin bool) ([]store.Pool, error) {
	p, err := store.SharedPool(ctx, s.db(), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return []store.Pool{}, nil
	}
	if err != nil {
		return nil, err
	}
	if p.State != "open" && !admin {
		return []store.Pool{}, nil
	}
	return []store.Pool{p}, nil
}
func (s *Service) CreateApplication(ctx context.Context, actor string, in ApplicationInput) (store.Case, error) {
	if in.PoolID <= 0 || in.BorrowerCharacterID <= 0 || in.PrincipalMinor <= 0 || in.InterestMinor < 0 || in.InstallmentCount < 1 || in.InstallmentCount > 120 || in.IntervalDays < 1 || in.IntervalDays > 365 {
		return store.Case{}, ErrInvalid
	}
	first, e := time.Parse(time.RFC3339, in.FirstDueAt)
	if e != nil || first.Before(time.Now().UTC().Add(-time.Minute)) {
		return store.Case{}, ErrInvalid
	}
	if s.Characters == nil {
		return store.Case{}, ErrRule
	}
	chars, e := s.Characters(ctx, actor)
	if e != nil {
		return store.Case{}, e
	}
	found := false
	for _, c := range chars {
		if c.ID == in.BorrowerCharacterID {
			found = true
		}
	}
	if !found {
		return store.Case{}, ErrForbidden
	}
	p, e := store.GetPool(ctx, s.db(), in.PoolID)
	if e != nil {
		return store.Case{}, e
	}
	if p.State != "open" {
		return store.Case{}, ErrRule
	}
	if !p.IsShared {
		return store.Case{}, ErrRule
	}
	if p.LenderKind == "personal" && p.LenderUserID != nil && *p.LenderUserID == actor {
		return store.Case{}, ErrForbidden
	}
	// A pool must explicitly declare its amount and schedule bounds. There is no
	// implicit interest, limit or haircut in the module.
	var cfg struct {
		MinPrincipal    int64 `json:"min_principal_minor"`
		MaxInstallments int   `json:"max_installments"`
	}
	if json.Unmarshal(p.Config, &cfg) != nil || cfg.MinPrincipal <= 0 || cfg.MaxInstallments <= 0 {
		return store.Case{}, ErrRule
	}
	if in.PrincipalMinor < cfg.MinPrincipal || in.InstallmentCount > cfg.MaxInstallments {
		return store.Case{}, ErrLimit
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return store.Case{}, e
	}
	defer tx.Rollback(ctx)
	c, e := store.CreateCase(ctx, tx, publicID("LND"), in.PoolID, actor, strconv.FormatInt(in.BorrowerCharacterID, 10), in.PrincipalMinor, in.InterestMinor, in.InstallmentCount, in.IntervalDays, first)
	if e != nil {
		return store.Case{}, e
	}
	rows := make([]store.Installment, 0, in.InstallmentCount)
	pp, pr := in.PrincipalMinor/int64(in.InstallmentCount), in.PrincipalMinor%int64(in.InstallmentCount)
	ip, ir := in.InterestMinor/int64(in.InstallmentCount), in.InterestMinor%int64(in.InstallmentCount)
	for n := 1; n <= in.InstallmentCount; n++ {
		pc, ic := pp, ip
		if n == in.InstallmentCount {
			pc += pr
			ic += ir
		}
		rows = append(rows, store.Installment{Sequence: n, DueAt: first.AddDate(0, 0, (n-1)*in.IntervalDays), PrincipalMinor: pc, InterestMinor: ic})
	}
	if e = store.CreateInstallments(ctx, tx, c.ID, rows); e != nil {
		return store.Case{}, e
	}
	if e = store.Audit(ctx, tx, c.ID, actor, "submitted", nil, c); e != nil {
		return store.Case{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return store.Case{}, e
	}
	return c, nil
}

func (s *Service) canManage(ctx context.Context, actor string, p store.Pool) (bool, error) {
	if s.IsAdministrator != nil {
		ok, e := s.IsAdministrator(ctx, actor)
		if e != nil {
			return false, e
		}
		if ok {
			return true, nil
		}
	}
	if p.LenderKind == "personal" && p.LenderUserID != nil && *p.LenderUserID == actor {
		return true, nil
	}
	if p.LenderKind == "corporation" && p.CorporationID != nil && s.CanManagePool != nil {
		return s.CanManagePool(ctx, actor, "corporation", *p.CorporationID)
	}
	return false, nil
}
func (s *Service) Review(ctx context.Context, actor string, id int64, in ReviewInput) (store.Case, error) {
	if id <= 0 || (in.State != "approved" && in.State != "rejected") || in.Version <= 0 || len(in.Note) > 1000 {
		return store.Case{}, ErrInvalid
	}
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return store.Case{}, e
	}
	if c.State != "submitted" {
		return store.Case{}, ErrConflict
	}
	p, e := store.GetPool(ctx, s.db(), c.PoolID)
	if e != nil {
		return store.Case{}, e
	}
	ok, e := s.canManage(ctx, actor, p)
	if e != nil || !ok {
		return store.Case{}, ErrForbidden
	}
	if actor == c.BorrowerAccountID {
		return store.Case{}, ErrForbidden
	}
	var evaluatedCredit store.Credit
	if in.State == "approved" {
		if p.IsShared {
			summary, se := store.Summary(ctx, s.db(), p.ID)
			if se != nil {
				return store.Case{}, se
			}
			if summary.AvailableMinor < c.PrincipalMinor {
				return store.Case{}, ErrLimit
			}
		}
		cr, e := s.Credit(ctx, c.BorrowerAccountID)
		if e != nil {
			return store.Case{}, ErrRule
		}
		if cr.State != "active" {
			return store.Case{}, ErrRule
		}
		evaluatedCredit = cr
		used, e := store.UsedResponsibility(ctx, s.db(), c.BorrowerAccountID)
		if e != nil {
			return store.Case{}, e
		}
		if used+c.PrincipalMinor > cr.TotalLimitMinor {
			return store.Case{}, ErrLimit
		}
		directUsed, e := store.UsedPrincipal(ctx, s.db(), c.BorrowerAccountID)
		if e != nil {
			return store.Case{}, e
		}
		unsecured := cr.UnsecuredLimitMinor - directUsed
		if unsecured < 0 {
			unsecured = 0
		}
		if c.PrincipalMinor > unsecured {
			cov, e := store.Coverage(ctx, s.db(), c.ID)
			if e != nil {
				return store.Case{}, e
			}
			if c.PrincipalMinor-unsecured > cov {
				return store.Case{}, ErrCoverage
			}
		}
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return store.Case{}, e
	}
	defer tx.Rollback(ctx)
	if in.State == "approved" && s.LockAccounts != nil {
		if e = s.guardAccounts(ctx, tx, c.BorrowerAccountID); e != nil {
			return store.Case{}, e
		}
		locked, le := store.GetCase(ctx, tx, id)
		if le != nil {
			return store.Case{}, le
		}
		if locked.Version != in.Version || locked.State != "submitted" {
			return store.Case{}, ErrConflict
		}
		c = locked
		p, e = store.GetPool(ctx, tx, c.PoolID)
		if e != nil {
			return store.Case{}, e
		}
		if p.IsShared {
			summary, se := store.Summary(ctx, tx, p.ID)
			if se != nil || summary.AvailableMinor < c.PrincipalMinor {
				if se != nil {
					return store.Case{}, se
				}
				return store.Case{}, ErrLimit
			}
		}
		evaluatedCredit, e = s.Credit(ctx, c.BorrowerAccountID)
		if e != nil || evaluatedCredit.State != "active" {
			return store.Case{}, ErrRule
		}
		used, ue := store.UsedResponsibility(ctx, tx, c.BorrowerAccountID)
		if ue != nil || used > evaluatedCredit.TotalLimitMinor-c.PrincipalMinor {
			return store.Case{}, ErrLimit
		}
		directUsed, ue := store.UsedPrincipal(ctx, tx, c.BorrowerAccountID)
		if ue != nil {
			return store.Case{}, ue
		}
		unsecured := evaluatedCredit.UnsecuredLimitMinor - directUsed
		if unsecured < 0 {
			unsecured = 0
		}
		if c.PrincipalMinor > unsecured {
			cov, ce := store.Coverage(ctx, tx, c.ID)
			if ce != nil {
				return store.Case{}, ce
			}
			if c.PrincipalMinor-unsecured > cov {
				return store.Case{}, ErrCoverage
			}
		}
	}
	out, e := store.TransitionCase(ctx, tx, id, in.Version, in.State, actor, in.Note)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return store.Case{}, ErrConflict
		}
		return store.Case{}, e
	}
	if in.State == "approved" {
		if e = s.recordCreditEvaluation(ctx, tx, evaluatedCredit); e != nil {
			return store.Case{}, e
		}
	}
	if e = store.Audit(ctx, tx, id, actor, "review", c, out); e != nil {
		return store.Case{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return store.Case{}, e
	}
	return out, nil
}

func contractMatches(c Contract, expected int64) bool {
	return contractamount.Matches(expected, c.Price) || contractamount.Matches(expected, c.Reward)
}

func hasParty(c Contract, characterID int64) bool {
	return characterID > 0 && (c.IssuerID == characterID || c.AssigneeID == characterID || c.AcceptorID == characterID)
}

func sameJSON(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	la, _ := json.Marshal(left)
	lb, _ := json.Marshal(right)
	return bytes.Equal(la, lb)
}

func (s *Service) contractOwnerMatchesPool(ctx context.Context, p store.Pool, kind string, owner int64) (bool, error) {
	if owner <= 0 {
		return false, nil
	}
	if p.LenderKind == "corporation" {
		return kind == "corporation" && p.CorporationID != nil && *p.CorporationID == owner, nil
	}
	if kind != "character" || p.LenderUserID == nil || s.Characters == nil {
		return false, nil
	}
	chars, err := s.Characters(ctx, *p.LenderUserID)
	if err != nil {
		return false, err
	}
	for _, ch := range chars {
		if ch.ID == owner {
			return true, nil
		}
	}
	return false, nil
}

// collateralMatchesPool requires the completed item contract to identify the
// configured custody target. A borrower-supplied contract is only collateral
// after the item transfer is addressed to the shared pool's custodian.
func collateralMatchesPool(p store.Pool, c Contract) bool {
	if p.LenderKind == "personal" && p.CustodianCharacterID != nil {
		return hasParty(c, *p.CustodianCharacterID)
	}
	if p.LenderKind == "corporation" && p.CorporationID != nil {
		corporationID := *p.CorporationID
		return c.ForCorporation && (c.AssigneeID == corporationID || c.AcceptorID == corporationID || c.IssuerCorporationID == corporationID)
	}
	return false
}

func (s *Service) CreatePayment(ctx context.Context, actor string, id int64, in PaymentInput) (store.Payment, error) {
	if id <= 0 || in.Version <= 0 || (in.Kind != "disbursement" && in.Kind != "repayment") || in.ContractKind != "character" && in.ContractKind != "corporation" || in.ContractOwnerID <= 0 || in.ContractID <= 0 || s.Contracts == nil {
		return store.Payment{}, ErrInvalid
	}
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return store.Payment{}, e
	}
	p, e := store.GetPool(ctx, s.db(), c.PoolID)
	if e != nil {
		return store.Payment{}, e
	}
	payer, recipient := "", ""
	recipientCharacter := c.BorrowerCharacterID
	if in.Kind == "disbursement" {
		if c.State != "approved" && c.State != "funding" {
			return store.Payment{}, ErrConflict
		}
		if p.LenderKind == "personal" && p.LenderUserID != nil {
			payer = *p.LenderUserID
		}
		recipient = c.BorrowerAccountID
	} else {
		if c.State != "active" || in.RecipientCharacterID <= 0 {
			return store.Payment{}, ErrConflict
		}
		payer = c.BorrowerAccountID
		if p.LenderKind == "personal" && p.LenderUserID != nil {
			recipient = *p.LenderUserID
		}
		recipientCharacter = in.RecipientCharacterID
	}
	if payer == actor || recipient == actor || in.Kind == "repayment" && actor == c.BorrowerAccountID { /* allowed participant */
	} else if in.Kind == "disbursement" {
		ok, e := s.canManage(ctx, actor, p)
		if e != nil || !ok {
			return store.Payment{}, ErrForbidden
		}
	} else {
		return store.Payment{}, ErrForbidden
	}
	expected := in.ExpectedMinor
	if in.Kind == "disbursement" {
		expected = c.PrincipalMinor
	} else if expected <= 0 || expected > c.TotalDueMinor {
		return store.Payment{}, ErrInvalid
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return store.Payment{}, e
	}
	defer tx.Rollback(ctx)
	payment, e := store.CreatePayment(ctx, tx, publicID("LNP"), id, in.Kind, payer, recipient, strconv.FormatInt(recipientCharacter, 10), expected, in.ContractKind, in.ContractOwnerID, in.ContractID, actor)
	if e != nil {
		return store.Payment{}, e
	}
	contract, e := s.Contracts.Read(ctx, actor, in.ContractKind, in.ContractOwnerID, in.ContractID)
	if e != nil {
		return store.Payment{}, e
	}
	if contract.Status != "finished" || !contractMatches(contract, payment.ExpectedMinor) || contract.Type == "courier" {
		return store.Payment{}, ErrContract
	}
	ownerMatches, e := s.contractOwnerMatchesPool(ctx, p, contract.OwnerKind, contract.OwnerID)
	if in.Kind == "disbursement" {
		if !ownerMatches || !hasParty(contract, c.BorrowerCharacterID) {
			return store.Payment{}, ErrContract
		}
	} else if contract.OwnerKind != "character" || contract.OwnerID != c.BorrowerCharacterID || !hasParty(contract, c.BorrowerCharacterID) || !hasParty(contract, in.RecipientCharacterID) {
		return store.Payment{}, ErrContract
	}
	if e = s.Contracts.Claim(ctx, tx, in.ContractID, "loan", id); e != nil {
		return store.Payment{}, e
	}
	evidence, _ := json.Marshal(map[string]any{"contract_id": contract.ID, "owner_kind": contract.OwnerKind, "owner_id": contract.OwnerID, "status": contract.Status, "price": contract.Price, "reward": contract.Reward, "completed": contract.Completed})
	payment, e = store.VerifyPayment(ctx, tx, payment.ID, payment.Version, evidence)
	if e != nil {
		return store.Payment{}, e
	}
	if in.Kind == "disbursement" {
		_, e = store.TransitionCase(ctx, tx, id, c.Version, "active", actor, "合同已核验放款")
	} else {
		inst, e2 := store.Installments(ctx, tx, id)
		if e2 != nil {
			return store.Payment{}, e2
		}
		remaining := payment.ExpectedMinor
		alloc := []struct{ ID, Principal, Interest int64 }{}
		for _, i := range inst {
			if remaining <= 0 {
				break
			}
			pneed := i.PrincipalMinor - i.PaidPrincipalMinor
			ineed := i.InterestMinor - i.PaidInterestMinor
			take := pneed
			if take > remaining {
				take = remaining
			}
			remaining -= take
			itake := ineed
			if itake > remaining {
				itake = remaining
			}
			remaining -= itake
			if take > 0 || itake > 0 {
				alloc = append(alloc, struct{ ID, Principal, Interest int64 }{i.ID, take, itake})
			}
		}
		if remaining > 0 {
			return store.Payment{}, ErrContract
		}
		if e = store.ApplyRepayment(ctx, tx, id, alloc); e != nil {
			return store.Payment{}, e
		}
		paid, e2 := store.CasePaid(ctx, tx, id)
		if e2 != nil {
			return store.Payment{}, e2
		}
		if paid {
			_, e = store.TransitionCase(ctx, tx, id, c.Version, "settled", actor, "合同已核验还款")
			if e == nil {
				e = store.ReleaseSecurity(ctx, tx, id)
			}
		}
	}
	if e != nil {
		return store.Payment{}, e
	}
	if e = store.Audit(ctx, tx, id, actor, "payment_verified", c, payment); e != nil {
		return store.Payment{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return store.Payment{}, e
	}
	return payment, nil
}

func (s *Service) AddGuarantee(ctx context.Context, actor string, id int64, in GuaranteeInput) (store.Guarantee, error) {
	if id <= 0 || in.AmountMinor <= 0 || in.GuarantorCharacterID <= 0 || len(in.Note) > 500 || !validCash(in.AmountMinor) {
		return store.Guarantee{}, ErrInvalid
	}
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return store.Guarantee{}, e
	}
	if actor != c.BorrowerAccountID || c.State != "submitted" && c.State != "awaiting_acceptance" {
		return store.Guarantee{}, ErrForbidden
	}
	if s.AccountForCharacter == nil {
		return store.Guarantee{}, ErrRule
	}
	guarantor, e := s.AccountForCharacter(ctx, in.GuarantorCharacterID)
	if e != nil || guarantor == "" {
		return store.Guarantee{}, ErrForbidden
	}
	if guarantor == actor || in.AmountMinor > c.PrincipalMinor {
		return store.Guarantee{}, ErrInvalid
	}
	g, e := store.AddGuarantee(ctx, s.db(), id, guarantor, in.GuarantorCharacterID, in.AmountMinor, in.Note)
	return g, e
}
func (s *Service) DecideGuarantee(ctx context.Context, actor string, id int64, in DecisionInput) (store.Guarantee, error) {
	if in.State != "accepted" && in.State != "rejected" || in.Version <= 0 {
		return store.Guarantee{}, ErrInvalid
	}
	g, err := store.GetGuarantee(ctx, s.db(), id)
	if err != nil {
		return g, err
	}
	if g.State != "invited" {
		return g, ErrConflict
	}
	if actor != g.GuarantorAccountID {
		return g, ErrForbidden
	}
	if in.State == "accepted" {
		caseRow, e := store.GetCase(ctx, s.db(), g.CaseID)
		if e != nil {
			return g, e
		}
		if actor == caseRow.BorrowerAccountID {
			return g, ErrForbidden
		}
		if caseRow.State != "submitted" && caseRow.State != "awaiting_acceptance" {
			return g, ErrConflict
		}
		credit, e := s.Credit(ctx, actor)
		if e != nil || credit.State != "active" {
			return g, ErrRule
		}
		tx, e := s.Pool.Begin(ctx)
		if e != nil {
			return g, e
		}
		defer tx.Rollback(ctx)
		if e = s.guardAccounts(ctx, tx, actor, caseRow.BorrowerAccountID); e != nil {
			return g, e
		}
		lockedCase, e := store.GetCase(ctx, tx, g.CaseID)
		if e != nil || (lockedCase.State != "submitted" && lockedCase.State != "awaiting_acceptance") {
			return g, ErrConflict
		}
		locked, e := store.GetGuarantee(ctx, tx, id)
		if e != nil || locked.Version != in.Version || locked.State != "invited" {
			return g, ErrConflict
		}
		used, e := store.UsedResponsibility(ctx, tx, actor)
		if e != nil || used > credit.TotalLimitMinor-g.AmountMinor {
			return g, ErrLimit
		}
		out, e := store.DecideGuarantee(ctx, tx, id, in.Version, in.State)
		if e != nil {
			return g, e
		}
		if e = store.Audit(ctx, tx, g.CaseID, actor, "guarantee_accepted", g, out); e != nil {
			return g, e
		}
		if e = tx.Commit(ctx); e != nil {
			return g, e
		}
		return out, nil
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return g, e
	}
	defer tx.Rollback(ctx)
	if e = s.guardAccounts(ctx, tx, actor); e != nil {
		return g, e
	}
	out, e := store.DecideGuarantee(ctx, tx, id, in.Version, in.State)
	if errors.Is(e, pgx.ErrNoRows) {
		return g, ErrConflict
	}
	if e != nil {
		return g, e
	}
	if e = store.Audit(ctx, tx, g.CaseID, actor, "guarantee_rejected", g, out); e != nil {
		return g, e
	}
	if e = tx.Commit(ctx); e != nil {
		return g, e
	}
	return out, nil
}
func (s *Service) AddCollateral(ctx context.Context, actor string, id int64, in CollateralInput) (store.Collateral, error) {
	if id <= 0 || in.OwnerAccountID != actor || in.ContractKind != "character" && in.ContractKind != "corporation" || in.ContractOwnerID <= 0 || in.ContractID <= 0 || in.ValuationMinor <= 0 || in.HaircutBPS < 1 || in.HaircutBPS > 10000 || in.CoveredMinor <= 0 {
		return store.Collateral{}, ErrInvalid
	}
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return store.Collateral{}, e
	}
	if actor != c.BorrowerAccountID || c.State == "settled" || c.State == "rejected" || c.State == "cancelled" || c.State == "defaulted" {
		return store.Collateral{}, ErrForbidden
	}
	if in.CoveredMinor > coveredByHaircut(in.ValuationMinor, in.HaircutBPS) {
		return store.Collateral{}, ErrCoverage
	}
	p, e := store.GetPool(ctx, s.db(), c.PoolID)
	if e != nil {
		return store.Collateral{}, e
	}
	if s.Contracts == nil {
		return store.Collateral{}, ErrRule
	}
	contract, e := s.Contracts.Read(ctx, actor, in.ContractKind, in.ContractOwnerID, in.ContractID)
	if e != nil || contract.OwnerKind != in.ContractKind || contract.OwnerID != in.ContractOwnerID || contract.Type != "item_exchange" || contract.Status != "finished" || !contract.ItemsReady || len(contract.Items) == 0 || !hasParty(contract, c.BorrowerCharacterID) || !collateralMatchesPool(p, contract) || !exactAmount(contract.Price, 0) || !exactAmount(contract.Reward, 0) {
		return store.Collateral{}, ErrContract
	}
	if len(in.Items) > 0 && !sameJSON(in.Items, contract.Items) {
		return store.Collateral{}, ErrContract
	}
	return store.AddCollateral(ctx, s.db(), id, actor, in.ContractKind, in.ContractOwnerID, in.ContractID, contract.Items, in.ValuationMinor, int64(in.HaircutBPS), in.CoveredMinor)
}

func coveredByHaircut(valuation int64, haircut int) int64 {
	if valuation <= 0 || haircut <= 0 {
		return 0
	}
	q, r := valuation/10000, valuation%10000
	return q*int64(haircut) + r*int64(haircut)/10000
}
func (s *Service) DecideCollateral(ctx context.Context, actor string, id int64, in DecisionInput) (store.Collateral, error) {
	if in.State != "approved" && in.State != "disputed" || in.Version <= 0 {
		return store.Collateral{}, ErrInvalid
	}
	c, err := store.GetCollateral(ctx, s.db(), id)
	if err != nil {
		return c, err
	}
	if c.State != "proposed" {
		return c, ErrConflict
	}
	caseRow, e := store.GetCase(ctx, s.db(), c.CaseID)
	if e != nil {
		return c, e
	}
	p, e := store.GetPool(ctx, s.db(), caseRow.PoolID)
	if e != nil {
		return c, e
	}
	ok, e := s.canManage(ctx, actor, p)
	if e != nil || !ok {
		return c, ErrForbidden
	}
	if caseRow.State != "submitted" && caseRow.State != "awaiting_acceptance" && caseRow.State != "approved" {
		return c, ErrConflict
	}
	if in.State == "approved" {
		var cfg struct {
			CollateralHaircutBPS int `json:"collateral_haircut_bps"`
		}
		if json.Unmarshal(p.Config, &cfg) != nil || cfg.CollateralHaircutBPS < 1 || cfg.CollateralHaircutBPS > 10000 || c.HaircutBPS != cfg.CollateralHaircutBPS || c.CoveredMinor > coveredByHaircut(c.ValuationMinor, c.HaircutBPS) {
			return c, ErrRule
		}
		if s.Contracts == nil {
			return c, ErrRule
		}
		contract, e := s.Contracts.Read(ctx, actor, c.ContractKind, c.ContractOwnerID, c.ContractID)
		if e != nil || contract.OwnerKind != c.ContractKind || contract.OwnerID != c.ContractOwnerID || contract.Type != "item_exchange" || contract.Status != "finished" || !contract.ItemsReady || len(contract.Items) == 0 || !hasParty(contract, caseRow.BorrowerCharacterID) || !collateralMatchesPool(p, contract) || !exactAmount(contract.Price, 0) || !exactAmount(contract.Reward, 0) || !sameJSON(c.Items, contract.Items) {
			return c, ErrContract
		}
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if s.LockAccounts != nil {
		if e = s.guardAccounts(ctx, tx, caseRow.BorrowerAccountID); e != nil {
			return c, e
		}
	}
	if in.State == "approved" {
		if e = s.Contracts.Claim(ctx, tx, c.ContractID, "loan", c.CaseID); e != nil {
			return c, e
		}
	}
	out, e := store.DecideCollateral(ctx, tx, id, in.Version, in.State)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return c, ErrConflict
		}
		return c, e
	}
	if e = store.Audit(ctx, tx, c.CaseID, actor, "collateral_decision", c, out); e != nil {
		return c, e
	}
	if e = tx.Commit(ctx); e != nil {
		return c, e
	}
	return out, nil
}

func (s *Service) Detail(ctx context.Context, actor string, id int64) (map[string]any, error) {
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return nil, e
	}
	visible := actor == c.BorrowerAccountID
	if !visible {
		p, er := store.GetPool(ctx, s.db(), c.PoolID)
		if er != nil {
			return nil, er
		}
		visible, _ = s.canManage(ctx, actor, p)
		if !visible && c.LenderKind == "personal" && c.LenderUserID != nil {
			visible = *c.LenderUserID == actor
		}
	}
	if !visible {
		return nil, ErrForbidden
	}
	ins, e := store.Installments(ctx, s.db(), id)
	if e != nil {
		return nil, e
	}
	gs, e := store.ListGuarantees(ctx, s.db(), id)
	if e != nil {
		return nil, e
	}
	cs, e := store.ListCollateral(ctx, s.db(), id)
	if e != nil {
		return nil, e
	}
	return map[string]any{"case": c, "installments": ins, "guarantees": gs, "collateral": cs}, nil
}
func (s *Service) Guarantees(ctx context.Context, actor string) ([]store.Guarantee, error) {
	return store.ListGuaranteesForAccount(ctx, s.db(), actor)
}
func (s *Service) List(ctx context.Context, actor string, admin bool) ([]store.Case, error) {
	if admin {
		return store.ListCases(ctx, s.db(), actor, true)
	}
	pools, err := store.ListAllPools(ctx, s.db())
	if err != nil {
		return nil, err
	}
	corporations := []int64{}
	for _, p := range pools {
		if p.LenderKind != "corporation" || p.CorporationID == nil {
			continue
		}
		ok, e := s.canManage(ctx, actor, p)
		if e != nil {
			return nil, e
		}
		if ok {
			corporations = append(corporations, *p.CorporationID)
		}
	}
	return store.ListCasesForCorporations(ctx, s.db(), actor, corporations)
}

func queryInt(q url.Values, key string) (int64, error) {
	v := q.Get(key)
	if v == "" {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
