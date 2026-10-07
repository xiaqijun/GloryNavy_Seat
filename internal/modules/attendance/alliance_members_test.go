package attendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"glorynavy.local/seat/internal/testutil"
)

func TestAlliancePAPMembersReportAdminScopeAndBindingRecheck(t *testing.T) {
	ctx := context.Background()
	pool := testutil.Database(t)
	month := allianceMonth(time.Now().UTC())
	const admin = "00000000-0000-4000-8000-0000000000aa"
	const first = "00000000-0000-4000-8000-0000000000a1"
	const second = "00000000-0000-4000-8000-0000000000a2"
	const stale = "00000000-0000-4000-8000-0000000000a3"
	if _, err := pool.Exec(ctx, `
		INSERT INTO attendance_alliance_pap_snapshot(month,character_id,character_name,pap,account_id)
		VALUES ($1,101,'First Alt',2.50,$2),($1,102,'First Main',1.50,$2),
		       ($1,103,'Second Main',2.00,$3),($1,104,'Stale Character',9.00,$4)`, month, first, second, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE attendance_alliance_pap_sync
		SET month=$1,state='ready',complete=true,records_total=4,last_synced_at=now(),version=11
		WHERE singleton`, month); err != nil {
		t.Fatal(err)
	}

	s := &Service{
		Pool:          pool,
		Administrator: func(_ context.Context, user string) (bool, error) { return user == admin, nil },
		Bindings: func(_ context.Context, _ pgx.Tx, ids []int64) ([]Binding, error) {
			return []Binding{{ID: ids[0], UserID: first}, {ID: 102, UserID: first}, {ID: 103, UserID: second}}, nil
		},
		AlliancePAPMemberNames: func(_ context.Context, ids []string) (map[string]string, error) {
			return map[string]string{first: "First Member", second: "Second Member"}, nil
		},
	}

	if _, err := s.AlliancePAPMembersReport(ctx, first, month.Format("2006-01")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("non-admin report error = %v, want %v", err, pgx.ErrNoRows)
	}
	report, err := s.AlliancePAPMembersReport(ctx, admin, month.Format("2006-01"))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Available || report.State != "ready" || report.Version != 11 || report.RecordsTotal != 4 {
		t.Fatalf("report metadata = %+v", report)
	}
	if len(report.Members) != 2 {
		t.Fatalf("members = %+v, want two live accounts", report.Members)
	}
	if report.Members[0].UserID != first || report.Members[0].Name != "First Member" || report.Members[0].Points != 4 || !report.Members[0].Achieved || len(report.Members[0].Characters) != 2 {
		t.Fatalf("first member = %+v", report.Members[0])
	}
	if report.Members[1].UserID != second || report.Members[1].Points != 2 || report.Members[1].Achieved || len(report.Members[1].Characters) != 1 {
		t.Fatalf("second member = %+v", report.Members[1])
	}
}
