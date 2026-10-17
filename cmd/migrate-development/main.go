// migrate-development applies the checked-in PostgreSQL schema revisions to
// the explicitly marked Supabase development project. Production is rejected.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rnpproject/algentra-backend/internal/database"
)

type migration struct{ revision, file string }

var revisions = []migration{
	{"0001_initial_postgres_schema", "internal/database/migrations/0001_initial_postgres_schema.sql"},
	{"0002_rls_and_token_redaction", "internal/database/migrations/0002_rls_and_token_redaction.sql"},
	{"0003_midtrans_qris", "internal/database/migrations/0003_midtrans_qris.sql"},
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrasi development gagal:", err)
		os.Exit(1)
	}
}

func migrate(ctx context.Context) error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	values, err := readConfig(root)
	if err != nil {
		return err
	}
	processEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	if processEnv != "" && !strings.EqualFold(processEnv, "development") {
		return errors.New("migrasi hanya boleh dijalankan dengan APP_ENV=development")
	}
	if !strings.EqualFold(values["APP_ENV"], "development") {
		return errors.New(".env.local harus secara eksplisit menetapkan APP_ENV=development")
	}
	ref := strings.TrimSpace(values["SUPABASE_DEV_PROJECT_REF"])
	if ref == "" {
		return errors.New("SUPABASE_DEV_PROJECT_REF wajib diisi")
	}
	runtimeURL, err := parseDatabaseURL(values["DATABASE_URL"], "algentra_api")
	if err != nil {
		return err
	}
	parsed, err := parseDatabaseURL(values["MIGRATION_DATABASE_URL"], "algentra_migrator")
	if err != nil {
		return err
	}
	if runtimeURL.Path != parsed.Path || runtimeURL.User.Username() == parsed.User.Username() ||
		!strings.Contains(strings.ToLower(runtimeURL.Host+" "+runtimeURL.User.Username()), strings.ToLower(ref)) ||
		!strings.Contains(strings.ToLower(parsed.Host+" "+parsed.User.Username()), strings.ToLower(ref)) {
		return errors.New("DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama dengan role berbeda")
	}
	config, err := pgx.ParseConfig(parsed.String())
	if err != nil {
		return errors.New("MIGRATION_DATABASE_URL tidak dapat diproses")
	}
	config.ConnectTimeout = 10 * time.Second
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return errors.New("database development tidak dapat dihubungi")
	}
	defer conn.Close(context.Background())
	if err = verifyProject(ctx, conn, ref); err != nil {
		return err
	}
	revision, appExists, err := currentRevision(ctx, conn)
	if err != nil {
		return err
	}
	if revision == "" && appExists {
		return errors.New("schema app sudah ada tanpa revision marker; migrasi otomatis ditolak")
	}
	start := -1
	if revision == "" {
		start = 0
	} else {
		for i, item := range revisions {
			if item.revision == revision {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return fmt.Errorf("revision database tidak dikenal: %s", revision)
		}
		if err = verifySchemaMarker(ctx, conn, revision); err != nil {
			return err
		}
	}
	if start == len(revisions) {
		fmt.Printf("Schema development sudah pada revision %s.\n", revision)
		return nil
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return errors.New("transaksi migrasi tidak dapat dimulai")
	}
	defer tx.Rollback(ctx)
	if revision == "" {
		if _, err = tx.Exec(ctx, `CREATE TABLE public.alembic_version (version_num VARCHAR(32) NOT NULL, CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num))`); err != nil {
			return errors.New("tabel revision tidak dapat dibuat; pastikan bootstrap development sudah dijalankan")
		}
	}
	var applied []string
	for _, item := range revisions[start:] {
		sqlText, e := database.ReadMigration(filepath.Base(item.file))
		if e != nil {
			return fmt.Errorf("file revision %s tidak dapat dibaca", item.revision)
		}
		if _, err = tx.Exec(ctx, string(sqlText), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("revision %s gagal diterapkan: %w", item.revision, err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM public.alembic_version`); err != nil {
			return errors.New("revision database tidak dapat diperbarui")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO public.alembic_version(version_num) VALUES($1)`, item.revision); err != nil {
			return errors.New("revision database tidak dapat disimpan")
		}
		applied = append(applied, item.revision)
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("transaksi migrasi tidak dapat disimpan")
	}
	for _, revision := range applied {
		fmt.Printf("Applied %s\n", revision)
	}
	if err = verifySchemaMarker(ctx, conn, revisions[len(revisions)-1].revision); err != nil {
		return err
	}
	return nil
}

func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("root project tidak ditemukan")
}

func readConfig(root string) (map[string]string, error) {
	file, err := os.Open(filepath.Join(root, ".env.local"))
	if err != nil {
		return nil, errors.New("file .env.local development tidak ditemukan")
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key != "APP_ENV" && key != "DATABASE_URL" && key != "MIGRATION_DATABASE_URL" && key != "SUPABASE_DEV_PROJECT_REF" {
			continue
		}
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	for _, key := range []string{"APP_ENV", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SUPABASE_DEV_PROJECT_REF"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			values[key] = value
		}
	}
	return values, nil
}

func parseDatabaseURL(raw, expectedRole string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql" && u.Scheme != "postgresql+psycopg") || u.Hostname() == "" || u.User == nil || u.Path == "" || u.Path == "/" {
		return nil, errors.New("MIGRATION_DATABASE_URL harus berupa connection string Supabase PostgreSQL lengkap")
	}
	if _, ok := u.User.Password(); !ok {
		return nil, errors.New("MIGRATION_DATABASE_URL harus memiliki password")
	}
	role := strings.ToLower(strings.SplitN(u.User.Username(), ".", 2)[0])
	if role != expectedRole {
		return nil, fmt.Errorf("URL database harus memakai role %s", expectedRole)
	}
	ssl := strings.ToLower(u.Query().Get("sslmode"))
	if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
		return nil, errors.New("MIGRATION_DATABASE_URL harus mewajibkan TLS lewat sslmode")
	}
	if u.Scheme == "postgresql+psycopg" {
		u.Scheme = "postgresql"
	}
	return u, nil
}

func verifyProject(ctx context.Context, conn *pgx.Conn, ref string) error {
	var marker *string
	if err := conn.QueryRow(ctx, `SELECT shobj_description(oid,'pg_database') FROM pg_database WHERE datname=current_database()`).Scan(&marker); err != nil {
		return errors.New("marker database development tidak dapat dibaca")
	}
	if marker == nil || *marker != "algentra:development:"+ref {
		return errors.New("migrasi ditolak: target bukan project development yang ditandai dengan benar")
	}
	return nil
}

func currentRevision(ctx context.Context, conn *pgx.Conn) (string, bool, error) {
	var tableExists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.alembic_version') IS NOT NULL`).Scan(&tableExists); err != nil {
		return "", false, err
	}
	var schemaExists bool
	if err := conn.QueryRow(ctx, `SELECT to_regnamespace('app') IS NOT NULL`).Scan(&schemaExists); err != nil {
		return "", false, err
	}
	if !tableExists {
		return "", schemaExists, nil
	}
	var version string
	err := conn.QueryRow(ctx, `SELECT version_num FROM public.alembic_version`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", schemaExists, nil
	}
	if err != nil {
		return "", schemaExists, errors.New("revision schema tidak dapat dibaca")
	}
	var extra bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.alembic_version WHERE version_num<>$1)`, version).Scan(&extra); err != nil || extra {
		return "", schemaExists, errors.New("database memiliki beberapa revision schema")
	}
	return version, schemaExists, nil
}

func verifySchemaMarker(ctx context.Context, conn *pgx.Conn, revision string) error {
	var marker *string
	if err := conn.QueryRow(ctx, `SELECT obj_description('app'::regnamespace,'pg_namespace')`).Scan(&marker); err != nil {
		return errors.New("marker revision schema app tidak dapat dibaca")
	}
	if marker == nil || *marker != "algentra:revision:"+revision {
		return errors.New("revision marker schema app tidak cocok dengan alembic_version")
	}
	return nil
}
