package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/app"
	"github.com/example/dddcqrs/internal/infrastructure/config"
	"github.com/example/dddcqrs/internal/infrastructure/persistence/outbox"
	"github.com/example/dddcqrs/internal/platform/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	i := app.Build(cfg)
	log := do.MustInvoke[*slog.Logger](i)

	pub, err := redis.NewPublisher(i)
	if err != nil {
		log.Error("publisher", "err", err)
		os.Exit(1)
	}
	defer func() { _ = pub.Close() }()

	relay := outbox.NewRelay(app.WritePoolFrom(i), pub, log)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := relay.Run(ctx); err != nil {
			log.Error("relay", "err", err)
		}
	}()
	log.Info("outbox relay started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancel()
	_ = i.Shutdown()
}
