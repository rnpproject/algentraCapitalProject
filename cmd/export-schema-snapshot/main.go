package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaHead = "0004_account_daily_profit"

type dbConfig struct{ projectRef, runtimeURL, migrationURL string }

func main() {
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), "Usage: export-schema-snapshot [destination.sql]") }
	flag.Parse()
	destination := ""
	if flag.NArg() > 0 {
		destination = flag.Arg(0)
	}
	if destination == "" {
		fmt.Fprintln(os.Stderr, "destination wajib diisi")
		os.Exit(2)
	}
	if err := export(destination); err != nil {
		fmt.Fprintln(os.Stderr, "export schema gagal:", err)
		os.Exit(1)
	}
}

func export(destination string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := loadDevelopmentConfig(filepath.Join(root, ".env.local"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.migrationURL)
	if err != nil {
		return fmt.Errorf("connection URL migrator tidak valid")
	}
	defer pool.Close()
	var projectMarker, schemaMarker, applied string
	if err = pool.QueryRow(ctx, `SELECT shobj_description(oid,'pg_database') FROM pg_database WHERE datname=current_database()`).Scan(&projectMarker); err != nil {
		return fmt.Errorf("tidak dapat memverifikasi marker project database")
	}
	if projectMarker != "algentra:development:"+cfg.projectRef {
		return errors.New("ekspor ditolak: marker Supabase tidak cocok dengan project development")
	}
	if err = pool.QueryRow(ctx, `SELECT version_num FROM public.alembic_version`).Scan(&applied); err != nil {
		return errors.New("ekspor ditolak: revisi migration database tidak tersedia")
	}
	if applied != schemaHead {
		return fmt.Errorf("ekspor ditolak: schema development belum berada pada head (expected=%s, applied=%s)", schemaHead, applied)
	}
	if err = pool.QueryRow(ctx, `SELECT obj_description('app'::regnamespace,'pg_namespace')`).Scan(&schemaMarker); err != nil {
		return errors.New("ekspor ditolak: marker revisi schema tidak tersedia")
	}
	if schemaMarker != "algentra:revision:"+schemaHead {
		return errors.New("ekspor ditolak: comment revision schema app tidak cocok dengan head")
	}
	pool.Close()
	return runDump(cfg.migrationURL, destination)
}

func loadDevelopmentConfig(envPath string) (dbConfig, error) {
	if strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) != "development" && strings.TrimSpace(os.Getenv("APP_ENV")) != "" {
		return dbConfig{}, errors.New("operasi database ini hanya boleh dijalankan dengan APP_ENV=development")
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return dbConfig{}, errors.New("file .env.local development tidak ditemukan")
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		switch name {
		case "APP_ENV", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SUPABASE_DEV_PROJECT_REF":
			value = strings.TrimSpace(value)
			if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
				value = value[1 : len(value)-1]
			}
			values[name] = value
		}
	}
	for _, name := range []string{"APP_ENV", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SUPABASE_DEV_PROJECT_REF"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			values[name] = v
		}
	}
	if !strings.EqualFold(values["APP_ENV"], "development") {
		return dbConfig{}, errors.New(".env.local harus secara eksplisit menetapkan APP_ENV=development untuk tooling schema")
	}
	ref := strings.TrimSpace(values["SUPABASE_DEV_PROJECT_REF"])
	runtimeURL := strings.TrimSpace(values["DATABASE_URL"])
	migrationURL := strings.TrimSpace(values["MIGRATION_DATABASE_URL"])
	if ref == "" || runtimeURL == "" || migrationURL == "" {
		return dbConfig{}, errors.New("SUPABASE_DEV_PROJECT_REF, DATABASE_URL, dan MIGRATION_DATABASE_URL wajib tersedia")
	}
	runtime, err := parsePostgresURL(runtimeURL, "DATABASE_URL")
	if err != nil {
		return dbConfig{}, err
	}
	migration, err := parsePostgresURL(migrationURL, "MIGRATION_DATABASE_URL")
	if err != nil {
		return dbConfig{}, err
	}
	runtimeRole := strings.ToLower(strings.SplitN(runtime.User.Username(), ".", 2)[0])
	migrationRole := strings.ToLower(strings.SplitN(migration.User.Username(), ".", 2)[0])
	if runtimeRole != "algentra_api" {
		return dbConfig{}, errors.New("DATABASE_URL harus memakai role algentra_api")
	}
	if migrationRole != "algentra_migrator" {
		return dbConfig{}, errors.New("MIGRATION_DATABASE_URL harus memakai role algentra_migrator")
	}
	if runtime.Path != migration.Path || runtime.User.Username() == migration.User.Username() || !strings.Contains(strings.ToLower(runtime.Host+" "+runtime.User.Username()), strings.ToLower(ref)) || !strings.Contains(strings.ToLower(migration.Host+" "+migration.User.Username()), strings.ToLower(ref)) {
		return dbConfig{}, errors.New("DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama")
	}
	return dbConfig{projectRef: ref, runtimeURL: runtime.String(), migrationURL: migration.String()}, nil
}

func parsePostgresURL(value, name string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%s tidak valid", name)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" && u.Scheme != "postgresql+psycopg" {
		return nil, fmt.Errorf("%s harus memakai Supabase PostgreSQL", name)
	}
	if u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Path == "/" || u.Path == "" {
		return nil, fmt.Errorf("%s harus berupa connection string PostgreSQL lengkap", name)
	}
	if _, ok := u.User.Password(); !ok {
		return nil, fmt.Errorf("%s harus berisi kredensial", name)
	}
	ssl := strings.ToLower(u.Query().Get("sslmode"))
	if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
		return nil, fmt.Errorf("%s harus mewajibkan TLS lewat sslmode", name)
	}
	if u.Scheme == "postgresql+psycopg" {
		u.Scheme = "postgresql"
	}
	return u, nil
}

func runDump(databaseURL, destination string) error {
	u, err := parsePostgresURL(databaseURL, "MIGRATION_DATABASE_URL")
	if err != nil {
		return err
	}
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return errors.New("pg_dump tidak ditemukan; pasang PostgreSQL client tools terlebih dahulu")
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), "algentra-schema-*.sql")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err = file.Close(); err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	username := u.User.Username()
	password, _ := u.User.Password()
	database := strings.TrimPrefix(u.Path, "/")
	query := u.Query()
	commandCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandCtx, pgDump, "--host", u.Hostname(), "--port", port, "--username", username, "--dbname", database, "--format=plain", "--schema-only", "--schema=app", "--no-owner", "--no-acl", "--file", temporary)
	command.Env = filteredEnvironment(os.Environ())
	command.Env = append(command.Env, "PGPASSWORD="+password, "PGSSLMODE="+query.Get("sslmode"), "PGCONNECT_TIMEOUT=10")
	if _, err := command.CombinedOutput(); err != nil {
		_ = os.Remove(temporary)
		return errors.New("pg_dump gagal; periksa koneksi dan izin role migrator")
	}
	contents, err := os.ReadFile(temporary)
	if err != nil {
		return err
	}
	if len(contents) == 0 {
		return errors.New("pg_dump menghasilkan file kosong")
	}
	contents = append(contents, []byte(fmt.Sprintf(`

-- Stamp only the verified Alembic head. This file contains schema DDL, not dev table data.
COMMENT ON SCHEMA app IS 'algentra:revision:%s';

GRANT USAGE ON SCHEMA app TO algentra_api, algentra_backup;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO algentra_api;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA app TO algentra_api;
GRANT SELECT ON ALL TABLES IN SCHEMA app TO algentra_backup;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA app TO algentra_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algentra_api;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO algentra_api;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT ON TABLES TO algentra_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT ON SEQUENCES TO algentra_backup;
`, schemaHead))...)
	if err = os.WriteFile(destination, contents, 0o600); err != nil {
		return err
	}
	fmt.Printf("Snapshot schema %s written to %s\n", schemaHead, destination)
	return nil
}

func filteredEnvironment(source []string) []string {
	out := make([]string, 0, len(source))
	for _, item := range source {
		name, _, _ := strings.Cut(item, "=")
		switch name {
		case "PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP":
			out = append(out, item)
		}
	}
	return out
}
