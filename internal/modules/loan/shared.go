package loan

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
)

// ISK entered in new custodial contracts is a whole number. Bound values also
// stay inside the frontend's exact integer range.
func validCash(amount int64) bool { return amount > 0 && amount%100 == 0 && amount <= 9007199254740900 }

type ContributionInput struct {
	LenderKind        string `json:"lender_kind"`
	SourceCharacterID int64  `json:"source_character_id"`
	CorporationID     int64  `json:"corporation_id"`
	AmountMinor       int64  `json:"amount_minor"`
}
type DepositInput struct {
	Version         int64  `json:"version"`
	ContractKind    string `json:"contract_kind"`
	ContractOwnerID int64  `json:"contract_owner_id"`
	ContractID      int64  `json:"contract_id"`
}
type cashParty struct {
	kind string
	id   int64
}

func partyEqual(a, b cashParty) bool { return a.kind == b.kind && a.id > 0 && a.id == b.id }
func exactAmount(value string, amount int64) bool {
	n, ok := new(big.Rat).SetString(value)
	return ok && n.Cmp(big.NewRat(amount, 100)) == 0
}

// A price is paid to the issuer; a reward is paid by the issuer. An assignee
// alone is not evidence of receipt. New pool entries require complete cash-only
// snapshots and a completed acceptor, with exactly one nonzero cash direction.
func cashMatches(c Contract, payer, receiver cashParty, amount int64) bool {
	if !validCash(amount) || c.Type != "item_exchange" || c.Status != "finished" || c.Completed == "" || !c.ItemsReady {
		return false
	}
	var items []json.RawMessage
	if json.Unmarshal(c.Items, &items) != nil || len(items) != 0 {
		return false
	}
	issuer := cashParty{"character", c.IssuerID}
	if c.ForCorporation {
		issuer = cashParty{"corporation", c.IssuerCorporationID}
	}
	if exactAmount(c.Price, amount) && exactAmount(c.Reward, 0) {
		return partyEqual(issuer, payer) && c.AcceptorID == receiver.id && c.AcceptorID > 0
	}
	if exactAmount(c.Reward, amount) && exactAmount(c.Price, 0) {
		return partyEqual(issuer, receiver) && c.AcceptorID == payer.id && c.AcceptorID > 0
	}
	return false
}
func custody(p store.Pool) cashParty {
	if p.CustodianCharacterID != nil {
		return cashParty{"character", *p.CustodianCharacterID}
	}
	return cashParty{}
}
func contributionParty(c store.Contribution) cashParty {
	if c.LenderKind == "corporation" && c.CorporationID != nil {
		return cashParty{"corporation", *c.CorporationID}
	}
	return cashParty{"character", c.SourceCharacterID}
}
func (s *Service) ownsCharacter(ctx context.Context, actor string, id int64) (bool, error) {
	if s.Characters == nil {
		return false, ErrRule
	}
	chars, e := s.Characters(ctx, actor)
	if e != nil {
		return false, e
	}
	for _, ch := range chars {
		if ch.ID == id {
			return true, nil
		}
	}
	return false, nil
}
func (s *Service) validCustody(ctx context.Context, p store.Pool) error {
	if p.LenderKind == "personal" && p.LenderUserID != nil && p.CustodianCharacterID != nil {
		ok, e := s.ownsCharacter(ctx, *p.LenderUserID, *p.CustodianCharacterID)
		if e != nil {
			return e
		}
		if ok {
			return nil
		}
	} else if p.LenderKind == "corporation" && p.CorporationID != nil {
		if p.CustodianCharacterID == nil || s.AccountForCharacter == nil {
			return ErrRule
		}
		if _, e := s.AccountForCharacter(ctx, *p.CustodianCharacterID); e != nil {
			return ErrRule
		}
		return nil
	}
	return ErrRule
}

func (s *Service) ConfigureCustodian(ctx context.Context, actor string, poolID, characterID, version int64) (store.Pool, error) {
	if poolID <= 0 || characterID <= 0 || version <= 0 || s.AccountForCharacter == nil {
		return store.Pool{}, ErrInvalid
	}
	p, err := store.SharedPool(ctx, s.db(), false)
	if err != nil {
		return p, err
	}
	if p.ID != poolID {
		return p, ErrForbidden
	}
	ok, err := s.canManage(ctx, actor, p)
	if err != nil || !ok {
		return p, ErrForbidden
	}
	if _, err = s.AccountForCharacter(ctx, characterID); err != nil {
		return p, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	if err = s.guardAccounts(ctx, tx, actor); err != nil {
		return p, err
	}
	locked, err := store.SharedPool(ctx, tx, true)
	if err != nil {
		return p, err
	}
	if locked.ID != poolID || locked.Version != version {
		return p, ErrConflict
	}
	out, err := store.UpdateCustodian(ctx, tx, poolID, characterID, version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, ErrConflict
		}
		return p, err
	}
	if err = store.Audit(ctx, tx, 0, actor, "custodian_updated", locked, out); err != nil {
		return p, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) guardAccounts(ctx context.Context, tx pgx.Tx, ids ...string) error {
	if s.LockAccounts == nil {
		return ErrRule
	}
	return s.LockAccounts(ctx, tx, ids)
}
func (s *Service) Contributions(ctx context.Context, actor string) ([]store.Contribution, error) {
	admin := false
	var e error
	if s.IsAdministrator != nil {
		admin, e = s.IsAdministrator(ctx, actor)
		if e != nil {
			return nil, e
		}
	}
	items, err := store.Contributions(ctx, s.db(), actor, admin)
	if err != nil || admin || s.Contracts == nil {
		return items, err
	}
	// Contract snapshots are already synchronized by the EVE module. Reading
	// contributions also advances pending deposits without requiring a UI action.
	for i, item := range items {
		if item.State != "pending" {
			continue
		}
		if updated, verifyErr := s.AutoVerifyContribution(ctx, actor, item.ID, item.Version); verifyErr == nil {
			items[i] = updated
		}
	}
	return items, nil
}
func (s *Service) canContribute(ctx context.Context, actor string, c store.Contribution) error {
	if actor != c.AccountID {
		if s.IsAdministrator == nil {
			return ErrForbidden
		}
		ok, e := s.IsAdministrator(ctx, actor)
		if e != nil || !ok {
			return ErrForbidden
		}
	}
	// Administrative read access does not delegate the corporation's funding write.
	if c.LenderKind == "corporation" {
		if c.CorporationID == nil || s.CanManagePool == nil {
			return ErrForbidden
		}
		ok, e := s.CanManagePool(ctx, actor, "corporation", *c.CorporationID)
		if e != nil || !ok {
			return ErrForbidden
		}
	}
	ok, e := s.ownsCharacter(ctx, c.AccountID, c.SourceCharacterID)
	if e != nil {
		return e
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s *Service) AddContribution(ctx context.Context, actor string, in ContributionInput) (store.Contribution, error) {
	if !validCash(in.AmountMinor) || in.SourceCharacterID <= 0 || in.LenderKind != "personal" && in.LenderKind != "corporation" || in.LenderKind == "personal" && in.CorporationID != 0 || in.LenderKind == "corporation" && in.CorporationID <= 0 {
		return store.Contribution{}, ErrInvalid
	}
	c := store.Contribution{AccountID: actor, LenderKind: in.LenderKind, SourceCharacterID: in.SourceCharacterID}
	if in.CorporationID > 0 {
		c.CorporationID = &in.CorporationID
	}
	if e := s.canContribute(ctx, actor, c); e != nil {
		return c, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if e = s.guardAccounts(ctx, tx, actor); e != nil {
		return c, e
	}
	p, e := store.SharedPool(ctx, tx, true)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrRule
	}
	if e != nil {
		return c, e
	}
	if p.State != "open" {
		return c, ErrRule
	}
	c, e = store.AddContribution(ctx, tx, p.ID, actor, in.LenderKind, in.SourceCharacterID, in.CorporationID, in.AmountMinor)
	if e != nil {
		return c, e
	}
	if e = store.Audit(ctx, tx, 0, actor, "contribution_submitted", nil, c); e != nil {
		return c, e
	}
	return c, tx.Commit(ctx)
}
func (s *Service) CancelContribution(ctx context.Context, actor string, id int64, version int64) (store.Contribution, error) {
	if id <= 0 || version <= 0 {
		return store.Contribution{}, ErrInvalid
	}
	c, e := store.ContributionByID(ctx, s.db(), id)
	if e != nil {
		return c, e
	}
	if c.AccountID != actor {
		return c, ErrForbidden
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if e = s.guardAccounts(ctx, tx, actor); e != nil {
		return c, e
	}
	if _, e = store.SharedPool(ctx, tx, true); e != nil {
		return c, e
	}
	out, e := store.CancelContribution(ctx, tx, id, version)
	if errors.Is(e, pgx.ErrNoRows) {
		return c, ErrConflict
	}
	if e != nil {
		return c, e
	}
	if e = store.Audit(ctx, tx, 0, actor, "contribution_cancelled", c, out); e != nil {
		return c, e
	}
	return out, tx.Commit(ctx)
}
func (s *Service) VerifyContribution(ctx context.Context, actor string, id int64, in DepositInput) (store.Contribution, error) {
	if id <= 0 || in.Version <= 0 || in.ContractID <= 0 || in.ContractOwnerID <= 0 || in.ContractKind != "character" && in.ContractKind != "corporation" || s.Contracts == nil || s.ReadContractTx == nil {
		return store.Contribution{}, ErrInvalid
	}
	c, e := store.ContributionByID(ctx, s.db(), id)
	if e != nil {
		return c, e
	}
	if e = s.canContribute(ctx, actor, c); e != nil {
		return c, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	if e = s.guardAccounts(ctx, tx, actor, c.AccountID); e != nil {
		return c, e
	}
	p, e := store.SharedPool(ctx, tx, true)
	if e != nil {
		return c, e
	}
	c, e = store.ContributionByID(ctx, tx, id)
	if e != nil {
		return c, e
	}
	if c.State != "pending" || c.Version != in.Version || c.PoolID != p.ID {
		return c, ErrConflict
	}
	if e = s.validCustody(ctx, p); e != nil {
		return c, e
	}
	source, target := contributionParty(c), custody(p)
	if partyEqual(source, target) {
		return c, ErrInvalid
	}
	contract, e := s.ReadContractTx(ctx, tx, actor, in.ContractKind, in.ContractOwnerID, in.ContractID)
	if e != nil {
		return c, e
	}
	if contract.ID != in.ContractID || contract.OwnerKind != in.ContractKind || contract.OwnerID != in.ContractOwnerID || !cashMatches(contract, source, target, c.AmountMinor) {
		return c, ErrContract
	}
	if e = s.Contracts.Claim(ctx, tx, contract.ID, "loan_deposit", c.ID); e != nil {
		return c, ErrConflict
	}
	evidence, _ := json.Marshal(contract)
	out, e := store.FundContribution(ctx, tx, id, in.Version, in.ContractKind, in.ContractOwnerID, in.ContractID, evidence)
	if e != nil {
		return c, e
	}
	if e = store.PoolLedger(ctx, tx, p.ID, c.ID, 0, c.AmountMinor, "deposit", actor); e != nil {
		return c, e
	}
	if e = store.Audit(ctx, tx, 0, actor, "contribution_funded", c, out); e != nil {
		return c, e
	}
	return out, tx.Commit(ctx)
}

// AutoVerifyContribution searches the current account's recent synchronised
// finished contracts, selects an exact cash-only match, and sends it through
// the same transactional verifier used by manual verification.
func (s *Service) AutoVerifyContribution(ctx context.Context, actor string, id, version int64) (store.Contribution, error) {
	if id <= 0 || version <= 0 || s.Contracts == nil {
		return store.Contribution{}, ErrInvalid
	}
	c, err := store.ContributionByID(ctx, s.db(), id)
	if err != nil {
		return c, err
	}
	if c.Version != version || c.State != "pending" {
		return c, ErrConflict
	}
	if err = s.canContribute(ctx, actor, c); err != nil {
		return c, err
	}
	p, err := store.SharedPool(ctx, s.db(), false)
	if err != nil {
		return c, err
	}
	candidates, err := s.Contracts.FindCash(ctx, actor, time.Now().UTC().Add(-14*24*time.Hour))
	if err != nil {
		return c, err
	}
	payer, receiver := contributionParty(c), custody(p)
	for _, candidate := range candidates {
		if cashMatches(candidate, payer, receiver, c.AmountMinor) {
			return s.VerifyContribution(ctx, actor, id, DepositInput{Version: version, ContractKind: candidate.OwnerKind, ContractOwnerID: candidate.OwnerID, ContractID: candidate.ID})
		}
	}
	return c, ErrContract
}

func (s *Service) createSharedPayment(ctx context.Context, actor string, id int64, in PaymentInput) (store.Payment, error) {
	if in.Version <= 0 || in.ContractID <= 0 || in.ContractOwnerID <= 0 || in.ContractKind != "character" && in.ContractKind != "corporation" || in.Kind != "disbursement" && in.Kind != "repayment" || s.ReadContractTx == nil || s.Contracts == nil {
		return store.Payment{}, ErrInvalid
	}
	c, e := store.GetCase(ctx, s.db(), id)
	if e != nil {
		return store.Payment{}, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return store.Payment{}, e
	}
	defer tx.Rollback(ctx)
	if e = s.guardAccounts(ctx, tx, actor, c.BorrowerAccountID); e != nil {
		return store.Payment{}, e
	}
	p, e := store.SharedPool(ctx, tx, true)
	if e != nil {
		return store.Payment{}, e
	}
	c, e = store.GetCase(ctx, tx, id)
	if e != nil {
		return store.Payment{}, e
	}
	if c.PoolID != p.ID || c.Version != in.Version {
		return store.Payment{}, ErrConflict
	}
	if e = s.validCustody(ctx, p); e != nil {
		return store.Payment{}, e
	}
	payer, receiver := custody(p), cashParty{"character", c.BorrowerCharacterID}
	amount := c.PrincipalMinor
	if in.Kind == "disbursement" {
		ok, e := s.canManage(ctx, actor, p)
		if e != nil || !ok || actor == c.BorrowerAccountID {
			return store.Payment{}, ErrForbidden
		}
		if c.State != "approved" {
			return store.Payment{}, ErrConflict
		}
		summary, e := store.Summary(ctx, tx, p.ID)
		if e != nil {
			return store.Payment{}, e
		}
		if summary.CashMinor < c.PrincipalMinor {
			return store.Payment{}, ErrLimit
		}
	} else {
		if actor != c.BorrowerAccountID {
			ok, e := s.canManage(ctx, actor, p)
			if e != nil || !ok {
				return store.Payment{}, ErrForbidden
			}
		}
		if c.State != "active" && c.State != "defaulted" {
			return store.Payment{}, ErrConflict
		}
		payer, receiver = receiver, payer
		amount = in.ExpectedMinor
	}
	if !validCash(amount) || partyEqual(payer, receiver) {
		return store.Payment{}, ErrInvalid
	}
	ok, e := s.ownsCharacter(ctx, c.BorrowerAccountID, c.BorrowerCharacterID)
	if e != nil || !ok {
		return store.Payment{}, ErrForbidden
	}
	contract, e := s.ReadContractTx(ctx, tx, actor, in.ContractKind, in.ContractOwnerID, in.ContractID)
	if e != nil {
		return store.Payment{}, e
	}
	if contract.ID != in.ContractID || contract.OwnerKind != in.ContractKind || contract.OwnerID != in.ContractOwnerID || !cashMatches(contract, payer, receiver, amount) {
		return store.Payment{}, ErrContract
	}
	if e = s.Contracts.Claim(ctx, tx, contract.ID, "loan", c.ID); e != nil {
		return store.Payment{}, ErrConflict
	}
	payerAccount, receiverAccount := c.BorrowerAccountID, ""
	if p.LenderUserID != nil {
		receiverAccount = *p.LenderUserID
	}
	if in.Kind == "disbursement" {
		payerAccount, receiverAccount = receiverAccount, payerAccount
	}
	payment, e := store.CreatePayment(ctx, tx, publicID("LNP"), id, in.Kind, payerAccount, receiverAccount, strconv.FormatInt(receiver.id, 10), amount, in.ContractKind, in.ContractOwnerID, in.ContractID, actor)
	if e != nil {
		return payment, e
	}
	evidence, _ := json.Marshal(contract)
	payment, e = store.VerifyPayment(ctx, tx, payment.ID, payment.Version, evidence)
	if e != nil {
		return payment, e
	}
	delta := amount
	if in.Kind == "disbursement" {
		delta = -amount
		_, e = store.TransitionCase(ctx, tx, id, c.Version, "active", actor, "合同已核验放款")
	} else {
		inst, err := store.Installments(ctx, tx, id)
		if err != nil {
			return payment, err
		}
		remaining := amount
		alloc := []struct{ ID, Principal, Interest int64 }{}
		for _, i := range inst {
			if remaining == 0 {
				break
			}
			pc := min(i.PrincipalMinor-i.PaidPrincipalMinor, remaining)
			remaining -= pc
			ic := min(i.InterestMinor-i.PaidInterestMinor, remaining)
			remaining -= ic
			if pc+ic > 0 {
				alloc = append(alloc, struct{ ID, Principal, Interest int64 }{i.ID, pc, ic})
			}
		}
		if remaining > 0 {
			return payment, ErrContract
		}
		if e = store.ApplyRepayment(ctx, tx, id, alloc); e != nil {
			return payment, e
		}
		paid, err := store.CasePaid(ctx, tx, id)
		if err != nil {
			return payment, err
		}
		if paid {
			_, e = store.TransitionCase(ctx, tx, id, c.Version, "settled", actor, "合同已核验还款")
			if e == nil {
				e = store.ReleaseSecurity(ctx, tx, id)
			}
		} else {
			_, e = tx.Exec(ctx, `UPDATE loan_cases SET version=version+1,updated_at=now() WHERE id=$1`, id)
		}
	}
	if e != nil {
		return payment, e
	}
	if e = store.PoolLedger(ctx, tx, p.ID, 0, payment.ID, delta, in.Kind, actor); e != nil {
		return payment, e
	}
	if e = store.Audit(ctx, tx, id, actor, "payment_verified", c, payment); e != nil {
		return payment, e
	}
	return payment, tx.Commit(ctx)
}
