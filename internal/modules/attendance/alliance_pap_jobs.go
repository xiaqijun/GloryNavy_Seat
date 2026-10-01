package attendance

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	jobs "glorynavy.local/seat/internal/platform/jobs"
)

type alliancePAPDispatchArgs struct{}

func (alliancePAPDispatchArgs) Kind() string { return "attendance.alliance-pap.v1" }

type alliancePAPWorker struct {
	river.WorkerDefaults[alliancePAPDispatchArgs]
	service *Service
	enabled bool
}

func (w *alliancePAPWorker) Work(ctx context.Context, _ *river.Job[alliancePAPDispatchArgs]) error {
	if !w.enabled || w.service.AlliancePAP.URL == "" || w.service.AlliancePAP.AuthFile == "" {
		return river.JobSnooze(time.Hour)
	}
	bounded, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	return w.service.SyncAlliancePAP(bounded)
}

func alliancePAPPeriodic(service *Service, enabled bool) *river.PeriodicJob {
	return river.NewPeriodicJob(river.PeriodicInterval(30*time.Minute), func() (river.JobArgs, *river.InsertOpts) {
		return alliancePAPDispatchArgs{}, &river.InsertOpts{Queue: "attendance", UniqueOpts: jobs.ActiveUnique()}
	}, &river.PeriodicJobOpts{RunOnStart: enabled && service.AlliancePAP.URL != "" && service.AlliancePAP.AuthFile != ""})
}
