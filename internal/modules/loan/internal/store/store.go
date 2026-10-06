package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is intentionally private to loan. Other modules use the loan service,
// never these queries or tables.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Pool struct {
	IsShared             bool            `json:"is_shared"`
	CustodianCharacterID *int64          `json:"custodian_character_id,string"`
	ID                   int64           `json:"id,string"`
	LenderKind           string          `json:"lender_kind"`
	LenderUserID         *string         `json:"lender_user_id"`
	CorporationID        *int64          `json:"corporation_id,string"`
	Name                 string          `json:"name"`
	State                string          `json:"state"`
	Config               json.RawMessage `json:"config"`
	Version              int64           `json:"version"`
}

type Credit struct {
	AccountID           string    `json:"account_id"`
	Score               *int      `json:"score"`
	TotalLimitMinor     int64     `json:"total_limit_minor"`
	UnsecuredLimitMinor int64     `json:"unsecured_limit_minor"`
	State               string    `json:"state"`
	RuleVersion         string    `json:"rule_version"`
	Reason              string    `json:"reason"`
	Version             int64     `json:"version"`
	EvaluatedAt         time.Time `json:"evaluated_at"`
	SettledLoans        int       `json:"settled_loans"`
	ActiveLoans         int       `json:"active_loans"`
	DefaultedLoans      int       `json:"defaulted_loans"`
	PaidInstallments    int       `json:"paid_installments"`
	TotalInstallments   int       `json:"total_installments"`
	OverdueInstallments int       `json:"overdue_installments"`
	RepaymentPoints     int       `json:"repayment_points"`
	LeveragePoints      int       `json:"leverage_points"`
	SecurityPoints      int       `json:"security_points"`
	PAPPoints           int       `json:"pap_points"`
	AssetPoints         int       `json:"asset_points"`
	Evidence            string    `json:"evidence"`
}

type CreditSignals struct {
	SettledLoans        int
	ActiveLoans         int
	DefaultedLoans      int
	PaidInstallments    int
	TotalInstallments   int
	OverdueInstallments int
	OutstandingMinor    int64
	SecurityDisputes    int
}

type CreditEvaluation struct {
	ID                  int64
	AccountID           string
	PolicyVersion       string
	Score               int
	TotalLimitMinor     int64
	UnsecuredLimitMinor int64
	State               string
	FactorScores        json.RawMessage
	Evidence            json.RawMessage
	EvidenceCutoff      time.Time
	EvaluatedAt         time.Time
}

type Case struct {
	ID                  int64      `json:"id,string"`
	PublicID            string     `json:"public_id"`
	PoolID              int64      `json:"pool_id,string"`
	LenderKind          string     `json:"lender_kind"`
	LenderUserID        *string    `json:"lender_user_id"`
	CorporationID       *int64     `json:"corporation_id,string"`
	PoolName            string     `json:"pool_name"`
	BorrowerAccountID   string     `json:"borrower_account_id"`
	BorrowerCharacterID int64      `json:"borrower_character_id,string"`
	PrincipalMinor      int64      `json:"principal_minor"`
	InterestMinor       int64      `json:"interest_minor"`
	TotalDueMinor       int64      `json:"total_due_minor"`
	InstallmentCount    int        `json:"installment_count"`
	IntervalDays        int        `json:"interval_days"`
	FirstDueAt          time.Time  `json:"first_due_at"`
	State               string     `json:"state"`
	TermsVersion        int64      `json:"terms_version"`
	Version             int64      `json:"version"`
	ReviewerID          *string    `json:"reviewer_id"`
	ReviewNote          string     `json:"review_note"`
	CreatedAt           time.Time  `json:"created_at"`
	AcceptedAt          *time.Time `json:"accepted_at"`
	FundedAt            *time.Time `json:"funded_at"`
	SettledAt           *time.Time `json:"settled_at"`
}

type Installment struct {
	ID                 int64     `json:"id,string"`
	Sequence           int       `json:"sequence"`
	DueAt              time.Time `json:"due_at"`
	PrincipalMinor     int64     `json:"principal_minor"`
	InterestMinor      int64     `json:"interest_minor"`
	PaidPrincipalMinor int64     `json:"paid_principal_minor"`
	PaidInterestMinor  int64     `json:"paid_interest_minor"`
	State              string    `json:"state"`
}

type Payment struct {
	ID                   int64           `json:"id,string"`
	PublicID             string          `json:"public_id"`
	CaseID               int64           `json:"case_id,string"`
	Kind                 string          `json:"kind"`
	PayerAccountID       *string         `json:"payer_account_id"`
	RecipientAccountID   *string         `json:"recipient_account_id"`
	RecipientCharacterID int64           `json:"recipient_character_id,string"`
	ExpectedMinor        int64           `json:"expected_minor"`
	ContractKind         string          `json:"contract_kind"`
	ContractOwnerID      int64           `json:"contract_owner_id,string"`
	ContractID           int64           `json:"contract_id,string"`
	State                string          `json:"state"`
	Evidence             json.RawMessage `json:"evidence"`
	Version              int64           `json:"version"`
}

type Guarantee struct {
	ID                   int64  `json:"id,string"`
	CaseID               int64  `json:"case_id,string"`
	GuarantorAccountID   string `json:"guarantor_account_id"`
	GuarantorCharacterID int64  `json:"guarantor_character_id,string"`
	AmountMinor          int64  `json:"amount_minor"`
	State                string `json:"state"`
	Version              int64  `json:"version"`
	CasePublicID         string `json:"case_public_id,omitempty"`
	BorrowerAccountID    string `json:"borrower_account_id,omitempty"`
	BorrowerCharacterID  int64  `json:"borrower_character_id,string,omitempty"`
	PrincipalMinor       int64  `json:"principal_minor,omitempty"`
}

type Collateral struct {
	ID              int64           `json:"id,string"`
	CaseID          int64           `json:"case_id,string"`
	OwnerAccountID  string          `json:"owner_account_id"`
	ContractKind    string          `json:"contract_kind"`
	ContractOwnerID int64           `json:"contract_owner_id,string"`
	ContractID      int64           `json:"contract_id,string"`
	Items           json.RawMessage `json:"items"`
	ValuationMinor  int64           `json:"valuation_minor"`
	HaircutBPS      int             `json:"haircut_bps"`
	CoveredMinor    int64           `json:"covered_minor"`
	State           string          `json:"state"`
	Version         int64           `json:"version"`
}

func ListPools(ctx context.Context, db DBTX, user string) ([]Pool, error) {
	rows, err := db.Query(ctx, `SELECT id,lender_kind,lender_user_id,corporation_id,name,state,config,version,is_shared,custodian_character_id FROM loan_pools WHERE state='open' AND (lender_kind='personal' AND lender_user_id=$1::uuid OR lender_kind='corporation') ORDER BY id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pool{}
	for rows.Next() {
		var p Pool
		if err := rows.Scan(&p.ID, &p.LenderKind, &p.LenderUserID, &p.CorporationID, &p.Name, &p.State, &p.Config, &p.Version, &p.IsShared, &p.CustodianCharacterID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ListAllPools(ctx context.Context, db DBTX) ([]Pool, error) {
	rows, err := db.Query(ctx, `SELECT id,lender_kind,lender_user_id,corporation_id,name,state,config,version,is_shared,custodian_character_id FROM loan_pools WHERE state<>'closed' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pool{}
	for rows.Next() {
		var p Pool
		if err := rows.Scan(&p.ID, &p.LenderKind, &p.LenderUserID, &p.CorporationID, &p.Name, &p.State, &p.Config, &p.Version, &p.IsShared, &p.CustodianCharacterID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func GetPool(ctx context.Context, db DBTX, id int64) (Pool, error) {
	var p Pool
	err := db.QueryRow(ctx, `SELECT id,lender_kind,lender_user_id,corporation_id,name,state,config,version,is_shared,custodian_character_id FROM loan_pools WHERE id=$1`, id).Scan(&p.ID, &p.LenderKind, &p.LenderUserID, &p.CorporationID, &p.Name, &p.State, &p.Config, &p.Version, &p.IsShared, &p.CustodianCharacterID)
	return p, err
}

func CreditSignalsFor(ctx context.Context, db DBTX, account string) (CreditSignals, error) {
	var s CreditSignals
	err := db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state='settled'),
			(SELECT count(*) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state IN ('approved','funding','active')),
			(SELECT count(*) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state='defaulted'),
			(SELECT count(*) FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id WHERE c.borrower_account_id=$1::uuid AND i.state='paid'),
			(SELECT count(*) FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id WHERE c.borrower_account_id=$1::uuid),
			(SELECT count(*) FROM loan_installments i JOIN loan_cases c ON c.id=i.case_id WHERE c.borrower_account_id=$1::uuid AND i.state <> 'paid' AND i.due_at < now())
			,(SELECT COALESCE(sum(principal_minor),0) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state IN ('approved','funding','active','defaulted'))
			,(SELECT count(*) FROM loan_guarantees g JOIN loan_cases c ON c.id=g.case_id WHERE g.guarantor_account_id=$1::uuid AND g.state IN ('disputed','called'))
	`, account).Scan(&s.SettledLoans, &s.ActiveLoans, &s.DefaultedLoans, &s.PaidInstallments, &s.TotalInstallments, &s.OverdueInstallments, &s.OutstandingMinor, &s.SecurityDisputes)
	return s, err
}

func RecordCreditEvaluation(ctx context.Context, db DBTX, e CreditEvaluation) (CreditEvaluation, error) {
	var out CreditEvaluation
	err := db.QueryRow(ctx, `
        INSERT INTO loan_credit_evaluations(account_id,policy_version,score,total_limit_minor,unsecured_limit_minor,state,factor_scores,evidence,evidence_cutoff,evaluated_at)
        VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
        RETURNING id,account_id,policy_version,score,total_limit_minor,unsecured_limit_minor,state,factor_scores,evidence,evidence_cutoff,evaluated_at`,
		e.AccountID, e.PolicyVersion, e.Score, e.TotalLimitMinor, e.UnsecuredLimitMinor, e.State, e.FactorScores, e.Evidence, e.EvidenceCutoff, e.EvaluatedAt).
		Scan(&out.ID, &out.AccountID, &out.PolicyVersion, &out.Score, &out.TotalLimitMinor, &out.UnsecuredLimitMinor, &out.State, &out.FactorScores, &out.Evidence, &out.EvidenceCutoff, &out.EvaluatedAt)
	return out, err
}

func SecurityAccounts(ctx context.Context, db DBTX, caseID int64) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT guarantor_account_id::text FROM loan_guarantees WHERE case_id=$1 AND state IN ('accepted','active') ORDER BY guarantor_account_id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func CreateCase(ctx context.Context, db DBTX, public string, poolID int64, borrower, character string, principal, interest int64, count, interval int, first time.Time) (Case, error) {
	var c Case
	err := db.QueryRow(ctx, `INSERT INTO loan_cases(public_id,pool_id,borrower_account_id,borrower_character_id,principal_minor,interest_minor,total_due_minor,installment_count,interval_days,first_due_at) VALUES($1,$2,$3::uuid,$4,$5,$6,$5+$6,$7,$8,$9) RETURNING id,public_id,pool_id,borrower_account_id,borrower_character_id,principal_minor,interest_minor,total_due_minor,installment_count,interval_days,first_due_at,state,terms_version,version,reviewer_id,review_note,created_at,accepted_at,funded_at,settled_at`, public, poolID, borrower, character, principal, interest, count, interval, first).Scan(&c.ID, &c.PublicID, &c.PoolID, &c.BorrowerAccountID, &c.BorrowerCharacterID, &c.PrincipalMinor, &c.InterestMinor, &c.TotalDueMinor, &c.InstallmentCount, &c.IntervalDays, &c.FirstDueAt, &c.State, &c.TermsVersion, &c.Version, &c.ReviewerID, &c.ReviewNote, &c.CreatedAt, &c.AcceptedAt, &c.FundedAt, &c.SettledAt)
	return c, err
}

func CreateInstallments(ctx context.Context, db DBTX, caseID int64, rows []Installment) error {
	for _, i := range rows {
		if _, err := db.Exec(ctx, `INSERT INTO loan_installments(case_id,sequence,due_at,principal_minor,interest_minor) VALUES($1,$2,$3,$4,$5)`, caseID, i.Sequence, i.DueAt, i.PrincipalMinor, i.InterestMinor); err != nil {
			return err
		}
	}
	return nil
}

func GetCase(ctx context.Context, db DBTX, id int64) (Case, error) {
	var c Case
	err := db.QueryRow(ctx, `SELECT c.id,c.public_id,c.pool_id,p.lender_kind,p.lender_user_id,p.corporation_id,p.name,c.borrower_account_id,c.borrower_character_id,c.principal_minor,c.interest_minor,c.total_due_minor,c.installment_count,c.interval_days,c.first_due_at,c.state,c.terms_version,c.version,c.reviewer_id,c.review_note,c.created_at,c.accepted_at,c.funded_at,c.settled_at FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE c.id=$1`, id).Scan(&c.ID, &c.PublicID, &c.PoolID, &c.LenderKind, &c.LenderUserID, &c.CorporationID, &c.PoolName, &c.BorrowerAccountID, &c.BorrowerCharacterID, &c.PrincipalMinor, &c.InterestMinor, &c.TotalDueMinor, &c.InstallmentCount, &c.IntervalDays, &c.FirstDueAt, &c.State, &c.TermsVersion, &c.Version, &c.ReviewerID, &c.ReviewNote, &c.CreatedAt, &c.AcceptedAt, &c.FundedAt, &c.SettledAt)
	return c, err
}

func ListCases(ctx context.Context, db DBTX, account string, all bool) ([]Case, error) {
	query := `SELECT c.id,c.public_id,c.pool_id,p.lender_kind,p.lender_user_id,p.corporation_id,p.name,c.borrower_account_id,c.borrower_character_id,c.principal_minor,c.interest_minor,c.total_due_minor,c.installment_count,c.interval_days,c.first_due_at,c.state,c.terms_version,c.version,c.reviewer_id,c.review_note,c.created_at,c.accepted_at,c.funded_at,c.settled_at FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE c.borrower_account_id=$1::uuid OR p.lender_kind='personal' AND p.lender_user_id=$1::uuid ORDER BY c.created_at DESC`
	args := []any{account}
	if all {
		query = strings.Replace(query, "WHERE c.borrower_account_id=$1::uuid OR p.lender_kind='personal' AND p.lender_user_id=$1::uuid", "", 1)
		args = []any{}
	}
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Case{}
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.PublicID, &c.PoolID, &c.LenderKind, &c.LenderUserID, &c.CorporationID, &c.PoolName, &c.BorrowerAccountID, &c.BorrowerCharacterID, &c.PrincipalMinor, &c.InterestMinor, &c.TotalDueMinor, &c.InstallmentCount, &c.IntervalDays, &c.FirstDueAt, &c.State, &c.TermsVersion, &c.Version, &c.ReviewerID, &c.ReviewNote, &c.CreatedAt, &c.AcceptedAt, &c.FundedAt, &c.SettledAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func ListCasesForCorporations(ctx context.Context, db DBTX, account string, corporations []int64) ([]Case, error) {
	if len(corporations) == 0 {
		return ListCases(ctx, db, account, false)
	}
	rows, err := db.Query(ctx, `SELECT c.id,c.public_id,c.pool_id,p.lender_kind,p.lender_user_id,p.corporation_id,p.name,c.borrower_account_id,c.borrower_character_id,c.principal_minor,c.interest_minor,c.total_due_minor,c.installment_count,c.interval_days,c.first_due_at,c.state,c.terms_version,c.version,c.reviewer_id,c.review_note,c.created_at,c.accepted_at,c.funded_at,c.settled_at FROM loan_cases c JOIN loan_pools p ON p.id=c.pool_id WHERE c.borrower_account_id=$1::uuid OR p.lender_kind='personal' AND p.lender_user_id=$1::uuid OR p.corporation_id=ANY($2::bigint[]) ORDER BY c.created_at DESC`, account, corporations)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Case{}
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.PublicID, &c.PoolID, &c.LenderKind, &c.LenderUserID, &c.CorporationID, &c.PoolName, &c.BorrowerAccountID, &c.BorrowerCharacterID, &c.PrincipalMinor, &c.InterestMinor, &c.TotalDueMinor, &c.InstallmentCount, &c.IntervalDays, &c.FirstDueAt, &c.State, &c.TermsVersion, &c.Version, &c.ReviewerID, &c.ReviewNote, &c.CreatedAt, &c.AcceptedAt, &c.FundedAt, &c.SettledAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func Installments(ctx context.Context, db DBTX, caseID int64) ([]Installment, error) {
	rows, e := db.Query(ctx, `SELECT id,sequence,due_at,principal_minor,interest_minor,paid_principal_minor,paid_interest_minor,state FROM loan_installments WHERE case_id=$1 ORDER BY sequence`, caseID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Installment{}
	for rows.Next() {
		var i Installment
		if e := rows.Scan(&i.ID, &i.Sequence, &i.DueAt, &i.PrincipalMinor, &i.InterestMinor, &i.PaidPrincipalMinor, &i.PaidInterestMinor, &i.State); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func UsedPrincipal(ctx context.Context, db DBTX, account string) (int64, error) {
	var n int64
	err := db.QueryRow(ctx, `SELECT COALESCE(sum(principal_minor),0) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state IN ('approved','funding','active','defaulted')`, account).Scan(&n)
	return n, err
}

// UsedResponsibility includes both the account's own active principal and
// accepted guarantees. It is used when a guarantor accepts a new obligation.
func UsedResponsibility(ctx context.Context, db DBTX, account string) (int64, error) {
	var n int64
	err := db.QueryRow(ctx, `SELECT
		COALESCE((SELECT sum(principal_minor) FROM loan_cases WHERE borrower_account_id=$1::uuid AND state IN ('approved','funding','active','defaulted')),0)
		+ COALESCE((SELECT sum(g.amount_minor) FROM loan_guarantees g JOIN loan_cases c ON c.id=g.case_id WHERE g.guarantor_account_id=$1::uuid AND g.state IN ('accepted','active') AND c.state IN ('approved','funding','active','defaulted')),0)`, account).Scan(&n)
	return n, err
}
func Coverage(ctx context.Context, db DBTX, caseID int64) (int64, error) {
	var g, k int64
	if err := db.QueryRow(ctx, `SELECT COALESCE(sum(amount_minor),0) FROM loan_guarantees WHERE case_id=$1 AND state IN ('accepted','active')`, caseID).Scan(&g); err != nil {
		return 0, err
	}
	if err := db.QueryRow(ctx, `SELECT COALESCE(sum(covered_minor),0) FROM loan_collateral WHERE case_id=$1 AND state IN ('approved','held')`, caseID).Scan(&k); err != nil {
		return 0, err
	}
	return g + k, nil
}

func TransitionCase(ctx context.Context, db DBTX, id, version int64, state, actor, note string) (Case, error) {
	var c Case
	err := db.QueryRow(ctx, `UPDATE loan_cases SET state=$2,reviewer_id=$3::uuid,review_note=$4,version=version+1,updated_at=now(),accepted_at=CASE WHEN $2='approved' THEN now() ELSE accepted_at END,funded_at=CASE WHEN $2='active' THEN now() ELSE funded_at END,settled_at=CASE WHEN $2='settled' THEN now() ELSE settled_at END WHERE id=$1 AND version=$5 RETURNING id,public_id,pool_id,borrower_account_id,borrower_character_id,principal_minor,interest_minor,total_due_minor,installment_count,interval_days,first_due_at,state,terms_version,version,reviewer_id,review_note,created_at,accepted_at,funded_at,settled_at`, id, state, actor, note, version).Scan(&c.ID, &c.PublicID, &c.PoolID, &c.BorrowerAccountID, &c.BorrowerCharacterID, &c.PrincipalMinor, &c.InterestMinor, &c.TotalDueMinor, &c.InstallmentCount, &c.IntervalDays, &c.FirstDueAt, &c.State, &c.TermsVersion, &c.Version, &c.ReviewerID, &c.ReviewNote, &c.CreatedAt, &c.AcceptedAt, &c.FundedAt, &c.SettledAt)
	return c, err
}

func CreatePayment(ctx context.Context, db DBTX, public string, caseID int64, kind, payer, recipient, character string, expected int64, contractKind string, owner, contractID int64, actor string) (Payment, error) {
	var p Payment
	err := db.QueryRow(ctx, `INSERT INTO loan_payment_intents(public_id,case_id,kind,payer_account_id,recipient_account_id,recipient_character_id,expected_minor,contract_kind,contract_owner_id,contract_id,created_by) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11::uuid) RETURNING id,public_id,case_id,kind,payer_account_id,recipient_account_id,recipient_character_id,expected_minor,contract_kind,contract_owner_id,contract_id,state,evidence,version`, public, caseID, kind, payer, recipient, character, expected, contractKind, owner, contractID, actor).Scan(&p.ID, &p.PublicID, &p.CaseID, &p.Kind, &p.PayerAccountID, &p.RecipientAccountID, &p.RecipientCharacterID, &p.ExpectedMinor, &p.ContractKind, &p.ContractOwnerID, &p.ContractID, &p.State, &p.Evidence, &p.Version)
	return p, err
}
func GetPayment(ctx context.Context, db DBTX, id int64) (Payment, error) {
	var p Payment
	err := db.QueryRow(ctx, `SELECT id,public_id,case_id,kind,payer_account_id,recipient_account_id,recipient_character_id,expected_minor,contract_kind,contract_owner_id,contract_id,state,evidence,version FROM loan_payment_intents WHERE id=$1`, id).Scan(&p.ID, &p.PublicID, &p.CaseID, &p.Kind, &p.PayerAccountID, &p.RecipientAccountID, &p.RecipientCharacterID, &p.ExpectedMinor, &p.ContractKind, &p.ContractOwnerID, &p.ContractID, &p.State, &p.Evidence, &p.Version)
	return p, err
}
func VerifyPayment(ctx context.Context, db DBTX, id, version int64, evidence json.RawMessage) (Payment, error) {
	var p Payment
	err := db.QueryRow(ctx, `UPDATE loan_payment_intents SET state='verified',evidence=$2,verified_at=now(),version=version+1 WHERE id=$1 AND version=$3 RETURNING id,public_id,case_id,kind,payer_account_id,recipient_account_id,recipient_character_id,expected_minor,contract_kind,contract_owner_id,contract_id,state,evidence,version`, id, evidence, version).Scan(&p.ID, &p.PublicID, &p.CaseID, &p.Kind, &p.PayerAccountID, &p.RecipientAccountID, &p.RecipientCharacterID, &p.ExpectedMinor, &p.ContractKind, &p.ContractOwnerID, &p.ContractID, &p.State, &p.Evidence, &p.Version)
	return p, err
}
func ApplyRepayment(ctx context.Context, db DBTX, caseID int64, allocations []struct{ ID, Principal, Interest int64 }) error {
	for _, a := range allocations {
		if _, err := db.Exec(ctx, `UPDATE loan_installments SET paid_principal_minor=paid_principal_minor+$2,paid_interest_minor=paid_interest_minor+$3,state=CASE WHEN paid_principal_minor+$2=principal_minor AND paid_interest_minor+$3=interest_minor THEN 'paid' ELSE 'partial' END,version=version+1 WHERE id=$1 AND case_id=$4`, a.ID, a.Principal, a.Interest, caseID); err != nil {
			return err
		}
	}
	return nil
}
func CasePaid(ctx context.Context, db DBTX, caseID int64) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM loan_installments WHERE case_id=$1 AND state<>'paid')`, caseID).Scan(&ok)
	return ok, err
}

func AddGuarantee(ctx context.Context, db DBTX, caseID int64, guarantor string, characterID int64, amount int64, note string) (Guarantee, error) {
	var g Guarantee
	err := db.QueryRow(ctx, `INSERT INTO loan_guarantees(case_id,guarantor_account_id,guarantor_character_id,amount_minor,note) VALUES($1,$2::uuid,$3,$4,$5) RETURNING id,case_id,guarantor_account_id,COALESCE(guarantor_character_id,0),amount_minor,state,version`, caseID, guarantor, characterID, amount, note).Scan(&g.ID, &g.CaseID, &g.GuarantorAccountID, &g.GuarantorCharacterID, &g.AmountMinor, &g.State, &g.Version)
	return g, err
}
func DecideGuarantee(ctx context.Context, db DBTX, id, version int64, state string) (Guarantee, error) {
	var g Guarantee
	err := db.QueryRow(ctx, `UPDATE loan_guarantees SET state=$2,version=version+1,decided_at=now() WHERE id=$1 AND version=$3 RETURNING id,case_id,guarantor_account_id,COALESCE(guarantor_character_id,0),amount_minor,state,version`, id, state, version).Scan(&g.ID, &g.CaseID, &g.GuarantorAccountID, &g.GuarantorCharacterID, &g.AmountMinor, &g.State, &g.Version)
	return g, err
}
func ListGuarantees(ctx context.Context, db DBTX, caseID int64) ([]Guarantee, error) {
	rows, e := db.Query(ctx, `SELECT id,case_id,guarantor_account_id,COALESCE(guarantor_character_id,0),amount_minor,state,version FROM loan_guarantees WHERE case_id=$1 ORDER BY id`, caseID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Guarantee{}
	for rows.Next() {
		var g Guarantee
		if e := rows.Scan(&g.ID, &g.CaseID, &g.GuarantorAccountID, &g.GuarantorCharacterID, &g.AmountMinor, &g.State, &g.Version); e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func GetGuarantee(ctx context.Context, db DBTX, id int64) (Guarantee, error) {
	var g Guarantee
	err := db.QueryRow(ctx, `SELECT id,case_id,guarantor_account_id,COALESCE(guarantor_character_id,0),amount_minor,state,version FROM loan_guarantees WHERE id=$1`, id).Scan(&g.ID, &g.CaseID, &g.GuarantorAccountID, &g.GuarantorCharacterID, &g.AmountMinor, &g.State, &g.Version)
	return g, err
}

func ListGuaranteesForAccount(ctx context.Context, db DBTX, account string) ([]Guarantee, error) {
	rows, err := db.Query(ctx, `SELECT g.id,g.case_id,g.guarantor_account_id,COALESCE(g.guarantor_character_id,0),g.amount_minor,g.state,g.version,c.public_id,c.borrower_account_id,c.borrower_character_id,c.principal_minor FROM loan_guarantees g JOIN loan_cases c ON c.id=g.case_id WHERE g.guarantor_account_id=$1::uuid ORDER BY g.id DESC`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Guarantee
	for rows.Next() {
		var g Guarantee
		if err := rows.Scan(&g.ID, &g.CaseID, &g.GuarantorAccountID, &g.GuarantorCharacterID, &g.AmountMinor, &g.State, &g.Version, &g.CasePublicID, &g.BorrowerAccountID, &g.BorrowerCharacterID, &g.PrincipalMinor); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func ReleaseSecurity(ctx context.Context, db DBTX, caseID int64) error {
	if _, err := db.Exec(ctx, `UPDATE loan_guarantees SET state='released',version=version+1,decided_at=now() WHERE case_id=$1 AND state IN ('accepted','active')`, caseID); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `UPDATE loan_collateral SET state='released',version=version+1,decided_at=now() WHERE case_id=$1 AND state IN ('approved','held')`, caseID)
	return err
}
func AddCollateral(ctx context.Context, db DBTX, caseID int64, owner, kind string, contractOwner, contractID int64, items json.RawMessage, valuation int64, haircut, covered int64) (Collateral, error) {
	var c Collateral
	err := db.QueryRow(ctx, `INSERT INTO loan_collateral(case_id,owner_account_id,contract_kind,contract_owner_id,contract_id,items,valuation_minor,haircut_bps,covered_minor) VALUES($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9) RETURNING id,case_id,owner_account_id,contract_kind,contract_owner_id,contract_id,items,valuation_minor,haircut_bps,covered_minor,state,version`, caseID, owner, kind, contractOwner, contractID, items, valuation, haircut, covered).Scan(&c.ID, &c.CaseID, &c.OwnerAccountID, &c.ContractKind, &c.ContractOwnerID, &c.ContractID, &c.Items, &c.ValuationMinor, &c.HaircutBPS, &c.CoveredMinor, &c.State, &c.Version)
	return c, err
}
func DecideCollateral(ctx context.Context, db DBTX, id, version int64, state string) (Collateral, error) {
	var c Collateral
	err := db.QueryRow(ctx, `UPDATE loan_collateral SET state=$2,version=version+1,decided_at=now() WHERE id=$1 AND version=$3 RETURNING id,case_id,owner_account_id,contract_kind,contract_owner_id,contract_id,items,valuation_minor,haircut_bps,covered_minor,state,version`, id, state, version).Scan(&c.ID, &c.CaseID, &c.OwnerAccountID, &c.ContractKind, &c.ContractOwnerID, &c.ContractID, &c.Items, &c.ValuationMinor, &c.HaircutBPS, &c.CoveredMinor, &c.State, &c.Version)
	return c, err
}
func ListCollateral(ctx context.Context, db DBTX, caseID int64) ([]Collateral, error) {
	rows, e := db.Query(ctx, `SELECT id,case_id,owner_account_id,contract_kind,contract_owner_id,contract_id,items,valuation_minor,haircut_bps,covered_minor,state,version FROM loan_collateral WHERE case_id=$1 ORDER BY id`, caseID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Collateral{}
	for rows.Next() {
		var c Collateral
		if e := rows.Scan(&c.ID, &c.CaseID, &c.OwnerAccountID, &c.ContractKind, &c.ContractOwnerID, &c.ContractID, &c.Items, &c.ValuationMinor, &c.HaircutBPS, &c.CoveredMinor, &c.State, &c.Version); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func GetCollateral(ctx context.Context, db DBTX, id int64) (Collateral, error) {
	var c Collateral
	err := db.QueryRow(ctx, `SELECT id,case_id,owner_account_id,contract_kind,contract_owner_id,contract_id,items,valuation_minor,haircut_bps,covered_minor,state,version FROM loan_collateral WHERE id=$1`, id).Scan(&c.ID, &c.CaseID, &c.OwnerAccountID, &c.ContractKind, &c.ContractOwnerID, &c.ContractID, &c.Items, &c.ValuationMinor, &c.HaircutBPS, &c.CoveredMinor, &c.State, &c.Version)
	return c, err
}

func Audit(ctx context.Context, db DBTX, caseID int64, actor, action string, before, after any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	_, err := db.Exec(ctx, `INSERT INTO loan_audit(case_id,actor_id,action,before_state,after_state) VALUES(NULLIF($1,0),$2::uuid,$3,$4,$5)`, caseID, actor, action, b, a)
	return err
}
