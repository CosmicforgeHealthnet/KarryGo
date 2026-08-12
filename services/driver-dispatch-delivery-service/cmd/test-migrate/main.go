package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DISPATCH_TEST_DATABASE_URL is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect Dispatch test database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping Dispatch test database: %v\n", err)
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join("migrations", "*.up.sql"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "list Dispatch migrations: %v\n", err)
		os.Exit(1)
	}
	sort.Strings(files)

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no Dispatch .up.sql migrations found")
		os.Exit(1)
	}

	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", file, err)
			os.Exit(1)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "begin %s: %v\n", filepath.Base(file), err)
			os.Exit(1)
		}

		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			fmt.Fprintf(os.Stderr, "apply %s: %v\n", filepath.Base(file), err)
			os.Exit(1)
		}

		if err := tx.Commit(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "commit %s: %v\n", filepath.Base(file), err)
			os.Exit(1)
		}
	}

	fmt.Printf("Dispatch test database migrations applied: %d files\n", len(files))
}
