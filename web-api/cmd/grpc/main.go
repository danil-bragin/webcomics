package main

import (
	"log/slog"
	"os"

	"github.com/samber/do/v2"

	"github.com/example/dddcqrs/internal/app"
	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/infrastructure/config"
	grpciface "github.com/example/dddcqrs/internal/interfaces/grpc"
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

	_ = grpciface.NewService(reg)

	// TODO: generate pb from api/proto, create grpc.Server, register the
	// generated UserServiceServer that delegates to grpciface.Service, then
	// Serve on cfg.GRPCPort. Skeleton stops here to avoid committing generated
	// code into the boilerplate.
	log.Info("grpc entry skeleton ready", "port", cfg.GRPCPort,
		"note", "generate pb and wire the server")
	_ = i
}
