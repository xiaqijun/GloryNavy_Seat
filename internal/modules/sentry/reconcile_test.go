package sentry

import (
	"context"
	"testing"
	"time"
)

type fakeAlertSettlement struct{ calls []string }

func (f *fakeAlertSettlement) ReserveAlertInterval(_ context.Context, grant, interval, key string, _, _ time.Time) error {
	f.calls = append(f.calls, "reserve:"+grant+":"+interval+":"+key)
	return nil
}
func (f *fakeAlertSettlement) SettleAlertInterval(_ context.Context, _, interval, _ string) error {
	f.calls = append(f.calls, "settle:"+interval)
	return nil
}
func (f *fakeAlertSettlement) ReleaseAlertInterval(_ context.Context, _, interval, _ string) error {
	f.calls = append(f.calls, "release:"+interval)
	return nil
}
func (f *fakeAlertSettlement) RefundAlertInterval(_ context.Context, _, interval, _ string) error {
	f.calls = append(f.calls, "refund:"+interval)
	return nil
}

func TestReconcileAlertDeliverySettlesTimeInterval(t *testing.T) {
	d := AlertDelivery{DeliveryID: "delivery-1", GrantID: "grant-1", ChargeEventID: "event-1", Revision: 2, StartedAt: "2026-10-01T00:00:00Z", EndedAt: "2026-10-01T00:00:30Z", DurationSeconds: 30, ConsumptionState: "consumed"}
	fake := &fakeAlertSettlement{}
	if err := ReconcileAlertDelivery(context.Background(), fake, d); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[1][:7] != "settle:" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}

func TestReconcileAlertDeliveryRejectsMismatchedDuration(t *testing.T) {
	d := AlertDelivery{DeliveryID: "delivery-1", GrantID: "grant-1", ChargeEventID: "event-1", Revision: 1, StartedAt: "2026-10-01T00:00:00Z", EndedAt: "2026-10-01T00:00:30Z", DurationSeconds: 31, ConsumptionState: "consumed"}
	if err := ReconcileAlertDelivery(context.Background(), &fakeAlertSettlement{}, d); err != ErrInvalidAlertDelivery {
		t.Fatalf("error = %v", err)
	}
}

func TestReconcileAlertDeliveryRefundDoesNotReserveAgain(t *testing.T) {
	d := AlertDelivery{DeliveryID: "delivery-1", GrantID: "grant-1", ChargeEventID: "event-1", Revision: 1, StartedAt: "2026-10-01T00:00:00Z", EndedAt: "2026-10-01T00:00:30Z", DurationSeconds: 30, ConsumptionState: "refunded"}
	fake := &fakeAlertSettlement{}
	if err := ReconcileAlertDelivery(context.Background(), fake, d); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || len(fake.calls[0]) < 7 || fake.calls[0][:7] != "refund:" {
		t.Fatalf("calls = %#v", fake.calls)
	}
}
