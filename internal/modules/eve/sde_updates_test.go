package eve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"glorynavy.local/seat/internal/modules/eve/internal/store"
	"glorynavy.local/seat/internal/testutil"
)

type fakeSDESource struct {
	build                  int64
	file                   string
	latestCalls, downloads int
	failure                error
	onDownload             func()
}

func (s *fakeSDESource) Latest(context.Context) (int64, error) {
	s.latestCalls++
	return s.build, s.failure
}
func (s *fakeSDESource) Download(context.Context, int64) (string, error) {
	s.downloads++
	if s.onDownload != nil {
		s.onDownload()
	}
	return s.file, nil
}

func TestSDEAutomaticUpdateLifecycle(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	first, err := ImportSDENames(ctx, pool, sdeFixture(t, 100, sampleTypes), 100)
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSDESource{build: 100}
	service := NewStaticData(pool, t.TempDir(), 6*time.Hour)
	service.source = source
	sync, err := NewSync(pool, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	sync.SetStaticData(service)
	for i := 0; i < 2; i++ {
		if err = sync.dispatch(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='eve.sde-update.v1' AND queue='eve_sde'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate SDE dispatch", count, err)
	}
	if err = service.update(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastStatus != "unchanged" || status.LastSuccessAt == nil || source.downloads != 0 || status.NextCheckAt.Before(time.Now().Add(5*time.Hour)) {
		t.Fatal("unchanged build downloaded or lost clock", status)
	}
	var snooze *river.JobSnoozeError
	if !errors.As(service.update(ctx), &snooze) || source.latestCalls != 1 {
		t.Fatal("restart ignored persistent due time")
	}
	due := func() {
		t.Helper()
		if _, e := pool.Exec(ctx, `UPDATE eve_sde_update_state SET next_check_at=now(),lease_until='epoch'`); e != nil {
			t.Fatal(e)
		}
	}
	due()
	source.build = 101
	source.file = sdeFixture(t, 101, strings.ReplaceAll(sampleTypes, "三钛合金", "新名称"))
	if err = service.update(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = service.Status(ctx)
	if status.ActiveBuild != 101 || status.LastStatus != "updated" || source.downloads != 1 {
		t.Fatal(status)
	}
	lastSuccess := *status.LastSuccessAt
	due()
	source.failure = errors.New("offline")
	if err = service.update(ctx); err == nil {
		t.Fatal("failure hidden")
	}
	status, _ = service.Status(ctx)
	if status.LastError != "metadata_failed" || status.FailureCount != 1 || status.ActiveBuild != 101 || !status.LastSuccessAt.Equal(lastSuccess) || time.Until(status.NextCheckAt) < 4*time.Minute || time.Until(status.NextCheckAt) > 6*time.Minute {
		t.Fatal("failure changed data or backoff", status)
	}
	source.failure = nil
	due()
	source.build = 102
	source.file = sdeFixture(t, 102, "{")
	if err = service.update(ctx); err == nil {
		t.Fatal("bad import succeeded")
	}
	status, _ = service.Status(ctx)
	if status.ActiveBuild != 101 || status.LastError != "import_failed" || status.FailureCount != 2 || time.Until(status.NextCheckAt) < 9*time.Minute {
		t.Fatal(status)
	}

	if err = ActivateSDENames(ctx, pool, first.ID); err != nil {
		t.Fatal(err)
	}
	calls := source.latestCalls
	if err = service.update(ctx); err != nil || calls != source.latestCalls {
		t.Fatal("pinned release triggered network", err)
	}
	if err = service.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	source.build = 101
	source.file = sdeFixture(t, 101, strings.ReplaceAll(sampleTypes, "三钛合金", "新名称"))
	if err = service.update(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = service.Status(ctx)
	if status.Pinned || status.ActiveBuild != 101 || status.FailureCount != 0 {
		t.Fatal("resume did not reactivate retained release", status)
	}
	due()
	source.build = 103
	source.file = sdeFixture(t, 103, sampleTypes)
	source.onDownload = func() {
		if err := ActivateSDENames(ctx, pool, first.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err = service.update(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = service.Status(ctx)
	if !status.Pinned || status.ActiveBuild != 100 || status.LastStatus != "pinned" {
		t.Fatal("in-flight download overwrote rollback", status)
	}
}

func TestSDEExpiredWorkerCannotPublish(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	old, err := store.ClaimSDEUpdate(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimSDEUpdate(ctx, pool); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("parallel lease accepted", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE eve_sde_update_state SET lease_until=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	newer, err := store.ClaimSDEUpdate(ctx, pool)
	if err != nil || newer <= old {
		t.Fatal(newer, err)
	}
	path := sdeFixture(t, 100, sampleTypes)
	if _, err = store.ImportSDENamesAutomatic(ctx, pool, path, 100, old); !errors.Is(err, store.ErrSDEUpdateSuperseded) {
		t.Fatal("old lease published", err)
	}
	if _, err = store.ImportSDENamesAutomatic(ctx, pool, path, 100, newer); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishSDEUpdate(ctx, pool, newer, 100, "updated", "", 6*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishSDEUpdate(ctx, pool, old, 99, "failed", "download_failed", 6*time.Hour); err != nil {
		t.Fatal(err)
	}
	state, err := store.SDEStatus(ctx, pool)
	if err != nil || state.LastStatus != "updated" || state.FailureCount != 0 || state.ObservedBuild != 100 {
		t.Fatal(state, err)
	}
}

func TestSDEQueueRunsWithoutSSOCredentials(t *testing.T) {
	pool := testutil.Database(t)
	sync, err := NewSync(pool, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatal(err)
	}
	service := NewStaticData(pool, t.TempDir(), time.Hour)
	service.source = &fakeSDESource{build: 100, file: sdeFixture(t, 100, sampleTypes)}
	sync.SetStaticData(service)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); sync.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("SDE worker did not stop")
		}
	}()
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("SDE did not run without SSO")
		case <-ticker.C:
			state, e := service.Status(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if state.ActiveBuild == 100 && state.LastStatus == "updated" {
				return
			}
		}
	}
}

func TestSDEOfficialSourceMetadataAndArchiveReuse(t *testing.T) {
	source := newSDESource(t.TempDir())
	body, err := os.ReadFile(sdeFixture(t, 100, sampleTypes))
	if err != nil {
		t.Fatal(err)
	}
	requests := []string{}
	source.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("Authorization") != "" || r.Header.Get("User-Agent") == "" || r.URL.Host != "developers.eveonline.com" {
			t.Fatal("invalid public SDE request")
		}
		data := string(body)
		if strings.HasSuffix(r.URL.Path, "latest.jsonl") {
			data = "{\"_key\":\"other\"}\n{\"_key\":\"sde\",\"buildNumber\":100}\n"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	build, err := source.Latest(context.Background())
	if err != nil || build != 100 {
		t.Fatal(build, err)
	}
	first, err := source.Download(context.Background(), build)
	if err != nil {
		t.Fatal(err)
	}
	second, err := source.Download(context.Background(), build)
	if err != nil || first != second || len(requests) != 2 {
		t.Fatal("archive downloaded twice", requests, err)
	}
	source.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{\"_key\":\"sde\",\"buildNumber\":1}\n{\"_key\":\"sde\",\"buildNumber\":2}"))}, nil
	})
	if _, err = source.Latest(context.Background()); err == nil {
		t.Fatal("duplicate build metadata accepted")
	}
}

func TestSDESameBuildMapperUpgrade(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprint(automatic), func(t *testing.T) {
			pool := testutil.Database(t)
			ctx := context.Background()
			file := sdeFixture(t, 100, sampleTypes)
			first, err := ImportSDENames(ctx, pool, file, 100)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE eve_sde_name_releases SET mapper_version=1,system_count=0 WHERE id=$1`, first.ID); err != nil {
				t.Fatal(err)
			}
			svc := NewStaticData(pool, t.TempDir(), time.Hour)
			src := &fakeSDESource{build: 100, file: file}
			svc.source = src
			if automatic {
				err = svc.update(ctx)
			} else {
				_, err = svc.UpdateLatest(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			status, err := svc.Status(ctx)
			if err != nil || status.MapperVersion != 2 || status.SystemCount != 2 || src.downloads != 1 {
				t.Fatal(status, err)
			}
		})
	}
}
