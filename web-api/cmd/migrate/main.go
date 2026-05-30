package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/example/dddcqrs/migrations"
)

func main() {
	cmd := flag.String("cmd", "up", "goose command: up|down|status|version")
	flag.Parse()

	// Migrations always run against the WRITE/master database.
	dsn := os.Getenv("WRITE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("WRITE_DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("dialect: %v", err)
	}
	if err := goose.Run(*cmd, db, "."); err != nil {
		log.Fatalf("goose %s: %v", *cmd, err)
	}
}
