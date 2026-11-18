package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/vzauartcc/roster-sync/internal/config"
	rostersync "github.com/vzauartcc/roster-sync/internal/sync"
	"github.com/vzauartcc/roster-sync/internal/vatusa"
	"github.com/vzauartcc/roster-sync/internal/zau"
)

const (
	syncSchedule = "*/10 * * * *"
	runTimeout   = 5 * time.Minute
)

func main() {
	slog.SetDefault(
		slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})),
	)

	cfg := config.Load()

	slog.Info("roster-sync starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal("failed to load timezone", err)
	}

	vatusaClient := vatusa.NewClient(cfg.VatusaAPIURL, cfg.VatusaAPIKey, nil)
	zauClient := zau.NewClient(cfg.ZauAPIURL, cfg.ZauAPIKey, nil)

	var runMutex sync.Mutex

	run := func() {
		runMutex.Lock()
		defer runMutex.Unlock()

		runCtx, runCancel := context.WithTimeout(ctx, runTimeout)
		defer runCancel()

		result, err := rostersync.Run(runCtx, vatusaClient, zauClient, time.Now())
		if err != nil {
			slog.Error("roster sync failed", "error", err)

			return
		}

		logCounts(result)
	}

	runner := cron.New(
		cron.WithChain(cron.Recover(cronLogger{})),
		cron.WithLocation(loc),
	)

	_, err = runner.AddFunc(syncSchedule, run)
	if err != nil {
		fatal("failed to schedule roster sync", err)
	}

	runner.Start()

	if cfg.LocalDevEnv != "" {
		slog.Info("invoked from script, running initial sync")

		zauClient = zau.NewClient("https://devapi.zauartcc.org", cfg.ZauAPIKey, nil)

		go run()
	} else {
		slog.Info("sleeping until next run")
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	<-sigs

	slog.Info("roster-sync shutting down")

	cancel()

	stopCtx := runner.Stop()

	<-stopCtx.Done()

	slog.Info("bye")
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func logCounts(result rostersync.Result) {
	attrs := make([]any, 0, 8)

	if result.Added > 0 {
		attrs = append(attrs, "added", result.Added)
	}

	if result.MadeMember > 0 {
		attrs = append(attrs, "madeMember", result.MadeMember)
	}

	if result.MadeVisitor > 0 {
		attrs = append(attrs, "madeVisitor", result.MadeVisitor)
	}

	if result.UpdatedCore > 0 {
		attrs = append(attrs, "updatedCore", result.UpdatedCore)
	}

	if result.UpdatedRating > 0 {
		attrs = append(attrs, "updatedRating", result.UpdatedRating)
	}

	if result.UpdatedRoles > 0 {
		attrs = append(attrs, "updatedRoles", result.UpdatedRoles)
	}

	if result.RemovedMember > 0 {
		attrs = append(attrs, "removedMember", result.RemovedMember)
	}

	if result.CertsRemoved > 0 {
		attrs = append(attrs, "certsRemoved", result.CertsRemoved)
	}

	if result.ACEGrants > 0 {
		attrs = append(attrs, "aceGrants", result.ACEGrants)
	}

	if len(attrs) == 0 {
		return
	}

	slog.Info("roster sync complete", attrs...)
}

type cronLogger struct{}

func (cronLogger) Info(msg string, keysAndValues ...any) {
	slog.Info(msg, keysAndValues...)
}

func (cronLogger) Error(err error, msg string, keysAndValues ...any) {
	slog.Error(msg, append(keysAndValues, "error", err)...)
}
