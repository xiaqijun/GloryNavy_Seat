// sde-import publishes type and solar-system names from an official JSONL archive.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/eve"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "SDE import failed:", err)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("file", "", "Local official JSONL ZIP")
	build := flag.Int64("build", 0, "Expected official SDE build number")
	latest := flag.Bool("latest", false, "Download the latest official Tranquility JSONL ZIP")
	activate := flag.Int64("activate", 0, "Explicitly reactivate an existing imported release ID")
	statusFlag := flag.Bool("status", false, "Show active SDE and automatic update status")
	resume := flag.Bool("resume", false, "Unpin the SDE version and make the next automatic check due")
	flag.Parse()
	modes := 0
	for _, mode := range []bool{*latest, *activate != 0, *statusFlag, *resume, *file != "" || *build != 0} {
		if mode {
			modes++
		}
	}
	if modes != 1 {
		return errors.New("choose exactly one of --latest, --file with --build, --activate, --status, --resume")
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if (*latest && (*file != "" || *build != 0)) || (*activate != 0 && (*latest || *file != "" || *build != 0)) {
		return errors.New("use --latest, --file with --build, or --activate")
	}
	if !*latest && !*statusFlag && !*resume && *activate == 0 && (*file == "" || *build <= 0) {
		return errors.New("use --latest or --file <zip> --build <number>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		return errors.New("database unavailable; check DATABASE_URL")
	}
	service := eve.NewStaticData(pool, os.Getenv("SDE_WORK_DIR"), 0)
	if *statusFlag {
		status, err := service.Status(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	if *resume {
		if err := service.Resume(ctx); err != nil {
			return err
		}
		fmt.Println("SDE version unpinned; next automatic check is due when SDE_AUTO_UPDATE is enabled.")
		return nil
	}
	if *activate != 0 {
		if err = eve.ActivateSDENames(ctx, pool, *activate); err != nil {
			return err
		}
		fmt.Printf("Activated SDE name release %d\n", *activate)
		return nil
	}
	var result eve.SDENamesRelease
	if *latest {
		result, err = service.UpdateLatest(ctx)
	} else {
		result, err = eve.ImportSDENames(ctx, pool, *file, *build)
	}
	if err != nil {
		return err
	}
	fmt.Printf("SDE names: release=%d build=%d types=%d systems=%d sha256=%s reused=%t\n", result.ID, result.Build, result.Count, result.SystemCount, result.SHA256, result.Reused)
	if result.Reused {
		fmt.Println("Existing release retained; use --activate <release> to change the active version explicitly.")
	}
	return nil
}
