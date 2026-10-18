package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ExpectedSchemaRevision = "0004_account_daily_profit"

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	parsedURL.Scheme = strings.ToLower(parsedURL.Scheme)
	if parsedURL.Scheme == "postgresql+psycopg" {
		parsedURL.Scheme = "postgresql"
		databaseURL = parsedURL.String()
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 30 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO app")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("buat pool PostgreSQL: %w", err)
	}
	return pool, nil
}

func CheckReady(ctx context.Context, pool *pgxpool.Pool, environment, projectRef string) error {
	if pool == nil {
		return errors.New("pool database belum dikonfigurasi")
	}
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database tidak tersedia: %w", err)
	}
	var projectMarker, schemaMarker sql.NullString
	if err := pool.QueryRow(ctx, `
		SELECT
		  shobj_description(db.oid, 'pg_database'),
		  obj_description(to_regnamespace('app'), 'pg_namespace')
		FROM pg_database AS db
		WHERE db.datname = current_database()
	`).Scan(&projectMarker, &schemaMarker); err != nil {
		return fmt.Errorf("baca marker database: %w", err)
	}
	wantProject := "algentra:" + environment + ":" + projectRef
	if !projectMarker.Valid || projectMarker.String != wantProject {
		return errors.New("database environment marker tidak cocok; menolak koneksi ke project yang salah")
	}
	wantSchema := "algentra:revision:" + ExpectedSchemaRevision
	if !schemaMarker.Valid || schemaMarker.String != wantSchema {
		return fmt.Errorf("revisi schema Supabase tidak cocok: expected=%s, applied=%v", wantSchema, schemaMarker.String)
	}
	tables := make([]string, 0, len(requiredColumns))
	for table := range requiredColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='app' AND table_name=$1`, table)
		if err != nil {
			return fmt.Errorf("periksa kolom schema app.%s: %w", table, err)
		}
		available := make(map[string]struct{}, len(requiredColumns[table]))
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return fmt.Errorf("baca kolom schema app.%s: %w", table, err)
			}
			available[name] = struct{}{}
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return fmt.Errorf("baca kolom schema app.%s: %w", table, rowErr)
		}
		if len(available) == 0 {
			return fmt.Errorf("schema Supabase belum lengkap: tabel app.%s tidak ditemukan", table)
		}
		missing := make([]string, 0)
		for _, name := range requiredColumns[table] {
			if _, exists := available[name]; !exists {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("schema Supabase belum kompatibel: kolom app.%s(%s) tidak ditemukan", table, strings.Join(missing, ", "))
		}
	}
	return nil
}
