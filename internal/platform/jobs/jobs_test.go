package jobs_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/platform/jobs"
	"glorynavy.local/seat/internal/testutil"
)

type rescueArgs struct {
	Long bool `json:"long"`
}

func (rescueArgs) Kind() string { return "test.rescue" }

type rescueWorker struct {
	river.WorkerDefaults[rescueArgs]
	completed chan bool
}

func (*rescueWorker) Timeout(j *river.Job[rescueArgs]) time.Duration {
	if j.Args.Long {
		return 16 * time.Minute
	}
	return time.Minute
}
func (w *rescueWorker) Work(_ context.Context, j *river.Job[rescueArgs]) error {
	w.completed <- j.Args.Long
	return nil
}

func TestRescueInterruptedScannerPreservesLongWorkerDeadline(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	worker := &rescueWorker{completed: make(chan bool, 2)}
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	client, err := jobs.New(pool, workers, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	short, err := client.Insert(ctx, rescueArgs{}, &river.InsertOpts{Queue: "eve_control"})
	if err != nil {
		t.Fatal(err)
	}
	long, err := client.Insert(ctx, rescueArgs{Long: true}, &river.InsertOpts{Queue: "eve_sde"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE river_job SET state='running',attempt=1,attempted_at=now()-interval '4 minutes' WHERE id=ANY($1::bigint[])`, []int64{short.Job.ID, long.Job.ID}); err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.StopAndCancel(stop); err != nil {
			t.Error(err)
		}
	}()
	select {
	case isLong := <-worker.completed:
		if isLong {
			t.Fatal("long worker rescued before its deadline")
		}
	case <-time.After(45 * time.Second):
		t.Fatal("interrupted scanner was not rescued")
	}
	row, err := client.JobGet(ctx, long.Job.ID)
	if err != nil || row.State != "running" || row.Attempt != 1 {
		t.Fatal("long worker state changed", err, row)
	}
}
