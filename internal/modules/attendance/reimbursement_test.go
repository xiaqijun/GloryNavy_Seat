package attendance

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestReimbursementConfirmedLossScope(t *testing.T) {
	s, _ := attendanceFixture(t)
	ctx := context.Background()
	var event int64
	err := s.Pool.QueryRow(ctx, `INSERT INTO attendance_events(corporation_id,title,starts_at,created_by,request_key) VALUES(10,'集结',now(),$1,$2) RETURNING id`, manager, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa").Scan(&event)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO attendance_entries(event_id,character_id,account_id,character_name,source,present,recorded_at) VALUES($1,4,$2,'Pilot','manual',true,now())`, event, member)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO attendance_losses(event_id,character_id,killmail_id,occurred_at,ship_type_id,solar_system_id,items) VALUES($1,4,900,now(),17715,30000142,'[]')`, event)
	if err != nil {
		t.Fatal(err)
	}
	check := func(owner string, corp, char int64, want bool) {
		t.Helper()
		rows, e := s.ConfirmedReimbursementLosses(ctx, owner, corp, char, []int64{900})
		if e != nil || (rows[900] > 0) != want {
			t.Fatalf("scope: %+v %v", rows, e)
		}
	}
	check(member, 10, 4, false)
	if _, err = s.Pool.Exec(ctx, `UPDATE attendance_losses SET state='confirmed' WHERE event_id=$1`, event); err != nil {
		t.Fatal(err)
	}
	check(member, 10, 4, true)
	check(manager, 10, 4, false)
	check(member, 20, 4, false)
	check(member, 10, 5, false)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.LockReimbursementLoss(ctx, tx, member, 10, 4, 900)
	if err != nil || id != event {
		t.Fatalf("guard %d %v", id, err)
	}
	_ = tx.Rollback(ctx)
	if _, err = s.Pool.Exec(ctx, `UPDATE attendance_entries SET present=false WHERE event_id=$1`, event); err != nil {
		t.Fatal(err)
	}
	check(member, 10, 4, false)
	tx, err = s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.LockReimbursementLoss(ctx, tx, member, 10, 4, 900); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("absent guard %v", err)
	}
}
