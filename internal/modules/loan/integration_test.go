package loan

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"glorynavy.local/seat/internal/modules/loan/internal/store"
	"glorynavy.local/seat/migrations"
)

func loanTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })
	schema := "loan_test_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.Files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestSubmitLoanApplicationAndContribution(t *testing.T) {
	db := loanTestDatabase(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow(ctx, `INSERT INTO identity_users DEFAULT VALUES RETURNING id::text`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	var poolID int64
	if err := db.QueryRow(ctx, `INSERT INTO loan_pools(lender_kind,corporation_id,name,state,config,created_by,is_shared)
VALUES('corporation',98000001,'Test loan pool','open','{"min_principal_minor":100,"max_installments":12}',$1::uuid,true) RETURNING id`, account).Scan(&poolID); err != nil {
		t.Fatal(err)
	}
	const characterID int64 = 2112345678
	service := &Service{Pool: db, Characters: func(context.Context, string) ([]Character, error) {
		return []Character{{ID: characterID}}, nil
	}, LockAccounts: func(context.Context, pgx.Tx, []string) error { return nil }}
	handler := Handler{Service: service, User: func(_ *http.Request) string { return account }}
	input := ApplicationInput{PoolID: poolID, BorrowerCharacterID: characterID, PrincipalMinor: 100_000_000, InstallmentCount: 3, IntervalDays: 30}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.applicationCreate(response, httptest.NewRequest("POST", "/api/v1/loan/applications", strings.NewReader(string(body))))
	if response.Code != 200 {
		t.Fatalf("submit application: status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data store.Case `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	application, err := store.GetCase(ctx, db, envelope.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	interest, err := simpleAnnualInterest(input.PrincipalMinor, 1200, 3, 30)
	if err != nil || application.BorrowerCharacterID != characterID || application.InterestMinor != interest || application.State != "submitted" || application.FirstDueAt != nil {
		t.Fatalf("application terms: %+v, interest error=%v", application, err)
	}
	var auditCount, installmentCount int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM loan_audit WHERE case_id=$1 AND action='submitted'),(SELECT count(*) FROM loan_installments WHERE case_id=$1)`, application.ID).Scan(&auditCount, &installmentCount); err != nil || auditCount != 1 || installmentCount != 0 {
		t.Fatalf("submission audit=%d installments=%d error=%v", auditCount, installmentCount, err)
	}
	input.BorrowerCharacterID++
	if _, err := service.CreateApplication(ctx, account, input); err != ErrForbidden {
		t.Fatalf("unbound character must be rejected: %v", err)
	}
	contribution, err := service.AddContribution(ctx, account, ContributionInput{LenderKind: "personal", SourceCharacterID: characterID, AmountMinor: 100_000_000})
	if err != nil {
		t.Fatal("submit contribution:", err)
	}
	summary, err := store.Summary(ctx, db, poolID)
	if err != nil || contribution.SourceCharacterID != characterID || contribution.State != "pending" || summary.PendingMinor != contribution.AmountMinor || summary.CashMinor != 0 {
		t.Fatalf("pending contribution=%+v summary=%+v error=%v", contribution, summary, err)
	}
	for i, kind := range []string{"disbursement", "repayment"} {
		t.Run(kind, func(t *testing.T) {
			payment, err := store.CreatePayment(ctx, db, publicID("LNP"), application.ID, kind, account, "", characterID, input.PrincipalMinor, "character", characterID, int64(70000001+i), account)
			if err != nil {
				t.Fatal("create payment:", err)
			}
			stored, err := store.GetPayment(ctx, db, payment.ID)
			if err != nil || stored.RecipientCharacterID != characterID || stored.ExpectedMinor != input.PrincipalMinor || stored.Kind != kind || stored.State != "pending" || stored.RecipientAccountID != nil {
				t.Fatalf("payment=%+v error=%v", stored, err)
			}
		})
	}
}
