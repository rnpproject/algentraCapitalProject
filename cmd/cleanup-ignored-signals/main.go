package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type counts struct{ signals, logs int64 }

const ignoredSignals = `SELECT COUNT(*) FROM signals s WHERE s.status='IGNORED' AND NOT EXISTS (SELECT 1 FROM trades t WHERE t.signal_id=s.id) AND NOT EXISTS (SELECT 1 FROM ea_executions e WHERE e.signal_id=s.id)`
const ignoredLogs = `SELECT COUNT(*) FROM logs WHERE source='Signal' AND message LIKE '%: IGNORED %'`
const deleteIgnoredSignals = `DELETE FROM signals AS s WHERE s.status='IGNORED' AND NOT EXISTS (SELECT 1 FROM trades t WHERE t.signal_id=s.id) AND NOT EXISTS (SELECT 1 FROM ea_executions e WHERE e.signal_id=s.id)`
const deleteIgnoredLogs = `DELETE FROM logs WHERE source='Signal' AND message LIKE '%: IGNORED %'`

func main() {
	apply := flag.Bool("apply", false, "hapus data (default: hanya hitung)")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "cleanup-ignored-signals tidak menerima argumen posisi")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openDatabase(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cleanup gagal:", err)
		os.Exit(1)
	}
	defer db.Close()
	result, err := cleanup(ctx, db, *apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cleanup gagal:", err)
		os.Exit(1)
	}
	label := "Akan dihapus (dry-run)"
	if *apply {
		label = "Dihapus"
	}
	fmt.Printf("%s: %d sinyal IGNORED, %d log\n", label, result.signals, result.logs)
}

func openDatabase(ctx context.Context) (*sql.DB, error) {
	value := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" {
		appEnv = strings.ToLower(strings.TrimSpace(readEnvValue(".env.local", "APP_ENV")))
	}
	if appEnv == "" {
		appEnv = "development"
	}
	if value == "" && appEnv != "production" {
		value = readEnvValue(".env.local", "DATABASE_URL")
	}
	if value == "" {
		return nil, errors.New("DATABASE_URL wajib diisi")
	}
	u, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("DATABASE_URL tidak valid")
	}
	var db *sql.DB
	switch strings.ToLower(u.Scheme) {
	case "sqlite":
		if appEnv == "production" {
			return nil, errors.New("production wajib memakai PostgreSQL")
		}
		path := u.Path
		if strings.HasPrefix(path, "//") {
			path = path[1:]
		} else {
			path = strings.TrimPrefix(path, "/")
		}
		if path == "" || strings.EqualFold(value, "sqlite://") || path == ":memory:" {
			path = ":memory:"
		}
		if path == "/" {
			return nil, errors.New("DATABASE_URL SQLite wajib menunjuk database")
		}
		if path == ":memory:" {
			path = ":memory:"
		} else {
			if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
				path = path[1:]
			}
			absolute, e := filepath.Abs(filepath.FromSlash(path))
			if e != nil {
				return nil, errors.New("DATABASE_URL SQLite tidak valid")
			}
			path = filepath.ToSlash(filepath.Clean(absolute))
		}
		if path == ":memory:" {
			db, err = sql.Open("sqlite", path)
			if err == nil {
				db.SetMaxOpenConns(1)
			}
		} else {
			uriPath := path
			if len(uriPath) >= 3 && uriPath[1] == ':' && uriPath[2] == '/' {
				uriPath = "/" + uriPath
			}
			dsn := (&url.URL{Scheme: "file", Path: uriPath}).String() + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
			db, err = sql.Open("sqlite", dsn)
		}
	case "postgres", "postgresql", "postgresql+psycopg":
		u.Scheme = "postgresql"
		if u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
			return nil, errors.New("DATABASE_URL PostgreSQL harus lengkap")
		}
		ssl := strings.ToLower(u.Query().Get("sslmode"))
		if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
			return nil, errors.New("DATABASE_URL PostgreSQL harus mewajibkan TLS")
		}
		if strings.SplitN(strings.ToLower(u.User.Username()), ".", 2)[0] != "algentra_api" {
			return nil, errors.New("DATABASE_URL harus memakai role runtime algentra_api")
		}
		projectRef := strings.TrimSpace(os.Getenv("SUPABASE_PROJECT_REF"))
		if appEnv != "production" {
			projectRef = strings.TrimSpace(os.Getenv("SUPABASE_DEV_PROJECT_REF"))
			if projectRef == "" {
				projectRef = strings.TrimSpace(readEnvValue(".env.local", "SUPABASE_DEV_PROJECT_REF"))
			}
			if projectRef == "" {
				return nil, errors.New("SUPABASE_DEV_PROJECT_REF wajib diisi untuk koneksi development PostgreSQL")
			}
		}
		identity := strings.ToLower(u.Hostname() + " " + u.User.Username())
		if projectRef != "" && !strings.Contains(identity, strings.ToLower(projectRef)) {
			return nil, errors.New("DATABASE_URL tidak cocok dengan project Supabase yang diharapkan")
		}
		cfg, e := pgx.ParseConfig(u.String())
		if e != nil {
			return nil, errors.New("DATABASE_URL PostgreSQL tidak valid")
		}
		db = stdlib.OpenDB(*cfg)
		db.SetMaxOpenConns(1)
	default:
		return nil, errors.New("DATABASE_URL harus memakai PostgreSQL atau SQLite")
	}
	if err != nil {
		return nil, fmt.Errorf("database tidak dapat dibuka: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("database tidak tersedia")
	}
	if strings.HasPrefix(strings.ToLower(value), "postgres") {
		if _, err = db.ExecContext(ctx, "SET search_path TO app"); err != nil {
			db.Close()
			return nil, errors.New("schema app tidak tersedia")
		}
	}
	return db, nil
}

func cleanup(ctx context.Context, db *sql.DB, apply bool) (counts, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return counts{}, err
	}
	defer tx.Rollback()
	var result counts
	if err = tx.QueryRowContext(ctx, ignoredSignals).Scan(&result.signals); err != nil {
		return counts{}, fmt.Errorf("hitung sinyal: %w", err)
	}
	if err = tx.QueryRowContext(ctx, ignoredLogs).Scan(&result.logs); err != nil {
		return counts{}, fmt.Errorf("hitung log: %w", err)
	}
	if apply {
		if _, err = tx.ExecContext(ctx, deleteIgnoredSignals); err != nil {
			return counts{}, fmt.Errorf("hapus sinyal: %w", err)
		}
		if _, err = tx.ExecContext(ctx, deleteIgnoredLogs); err != nil {
			return counts{}, fmt.Errorf("hapus log: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return counts{}, err
	}
	return result, nil
}

func readEnvValue(path, wanted string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != wanted {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		return value
	}
	return ""
}
