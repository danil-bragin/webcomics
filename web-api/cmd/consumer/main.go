package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/app"
	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/infrastructure/config"
	consumeriface "github.com/example/dddcqrs/internal/interfaces/consumer"
	wcredis "github.com/example/dddcqrs/internal/platform/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	i := app.Build(cfg)
	log := do.MustInvoke[*slog.Logger](i)
	reg := do.MustInvoke[*bus.Registry](i)

	sub, err := wcredis.NewSubscriber(i, consumeriface.ConsumerGroup)
	if err != nil {
		log.Error("subscriber", "err", err)
		os.Exit(1)
	}
	pub, err := wcredis.NewPublisher(i)
	if err != nil {
		log.Error("publisher", "err", err)
		os.Exit(1)
	}

	router, err := message.NewRouter(message.RouterConfig{}, watermill.NewSlogLogger(log))
	if err != nil {
		log.Error("router", "err", err)
		os.Exit(1)
	}

	consumer := consumeriface.New(reg, log)
	consumer.Register(router, sub)
	consumer.RegisterPipeline(router, sub, pub)

	// Optional run-lifecycle webhook (no-op when WEBHOOK_URL is empty).
	consumeriface.NewWebhookSender(cfg.WebhookURL, cfg.WebhookSecret, log).Register(router, sub, reg)

	// Raw go-redis loop for completion + failure streams (bypasses Watermill quirk).
	rc := do.MustInvoke[*wcredis.Client](i)
	raw := consumeriface.NewRawCompletionConsumer(rc.Client, reg, log)

	ctx, cancel := context.WithCancel(context.Background())
	raw.Run(ctx)
	go func() {
		if err := router.Run(ctx); err != nil {
			log.Error("router run", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancel()
	_ = router.Close()
	_ = i.Shutdown()
}
