package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var projectRefPattern = regexp.MustCompile(`^[a-z0-9]{10,30}$`)

func main() {
	validateOnly := flag.Bool("validate-only", false, "validate production database access without changing the database")
	flag.Parse()
	if err := setup(*validateOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func setup(validateOnly bool) error {
	adminURL := strings.TrimSpace(os.Getenv("DATABASE_ADMIN_URL"))
	projectRef := strings.ToLower(strings.TrimSpace(os.Getenv("SUPABASE_PROJECT_REF")))
	apiPassword := os.Getenv("ALGENTRA_API_PASSWORD")
	if adminURL == "" || !projectRefPattern.MatchString(projectRef) || len(apiPassword) < 32 {
		return errors.New("DATABASE_ADMIN_URL, SUPABASE_PROJECT_REF, atau password runtime tidak valid")
	}
	parsed, err := url.Parse(adminURL)
	if err != nil || parsed.User == nil || parsed.Hostname() == "" || strings.Trim(parsed.Path, "/") != "postgres" {
		return errors.New("DATABASE_ADMIN_URL harus menunjuk database postgres Supabase")
	}
	adminPassword, hasPassword := parsed.User.Password()
	if !hasPassword || adminPassword == "" {
		return errors.New("DATABASE_ADMIN_URL harus menyertakan password role postgres")
	}
	if port := parsed.Port(); port != "" && port != "5432" {
		return errors.New("DATABASE_ADMIN_URL harus memakai koneksi langsung atau session pooler Supabase pada port 5432; transaction pooler tidak cocok untuk service persisten")
	}
	adminUsername := parsed.User.Username()
	if !strings.EqualFold(strings.SplitN(adminUsername, ".", 2)[0], "postgres") {
		return errors.New("DATABASE_ADMIN_URL harus memakai role postgres project owner")
	}
	sslmode := strings.ToLower(parsed.Query().Get("sslmode"))
	if sslmode != "require" && sslmode != "verify-ca" && sslmode != "verify-full" {
		return errors.New("DATABASE_ADMIN_URL harus mewajibkan TLS lewat sslmode")
	}
	host := strings.ToLower(parsed.Hostname())
	directHost := host == "db."+projectRef+".supabase.co"
	poolerHost := strings.HasSuffix(host, ".pooler.supabase.com") && strings.HasSuffix(strings.ToLower(adminUsername), "."+projectRef)
	if !directHost && !poolerHost {
		return errors.New("DATABASE_ADMIN_URL tidak cocok dengan SUPABASE_PROJECT_REF")
	}

	schema, err := os.ReadFile("infra/supabase/fresh-production.sql")
	if err != nil {
		return fmt.Errorf("baca fresh production schema: %w", err)
	}
	schemaSQL := strings.ReplaceAll(string(schema), "REPLACE_WITH_SUPABASE_PROJECT_REF", projectRef)
	if strings.Contains(schemaSQL, "REPLACE_WITH_SUPABASE_PROJECT_REF") {
		return errors.New("project ref schema belum terganti")
	}
	passwordLiteral := "'" + strings.ReplaceAll(apiPassword, "'", "''") + "'"
	commit := "\nCOMMIT;"
	if !strings.Contains(schemaSQL, commit) {
		return errors.New("fresh production schema tidak memiliki batas COMMIT yang diharapkan")
	}
	schemaSQL = strings.Replace(schemaSQL, commit, "\nALTER ROLE algentra_api LOGIN PASSWORD "+passwordLiteral+";\nCOMMIT;", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		message := err.Error()
		message = strings.ReplaceAll(message, adminURL, "[DATABASE_ADMIN_URL]")
		if adminPassword != "" {
			message = strings.ReplaceAll(message, adminPassword, "[REDACTED]")
		}
		message = regexp.MustCompile(`(?i)(postgres(?:ql)?://)[^/@\s]+@`).ReplaceAllString(message, `${1}[REDACTED]@`)
		return fmt.Errorf("koneksi ke Supabase gagal: %s", message)
	}
	defer conn.Close(context.Background())
	runtimeURL := *parsed
	runtimeUser := "algentra_api"
	if strings.Contains(adminUsername, ".") {
		runtimeUser += "." + projectRef
	}
	runtimeURL.User = url.UserPassword(runtimeUser, apiPassword)
	var projectMarker, schemaMarker sql.NullString
	var appSchemaExists bool
	if err := conn.QueryRow(ctx, `
		SELECT shobj_description(db.oid, 'pg_database'),
		       obj_description(to_regnamespace('app'), 'pg_namespace'),
		       to_regnamespace('app') IS NOT NULL
		FROM pg_database AS db WHERE db.datname=current_database()
	`).Scan(&projectMarker, &schemaMarker, &appSchemaExists); err != nil {
		return errors.New("gagal memeriksa marker database target")
	}
	if validateOnly {
		if appSchemaExists && (!projectMarker.Valid || projectMarker.String != "algentra:production:"+projectRef || !schemaMarker.Valid || schemaMarker.String != "algentra:revision:0005_equity_candle_indexes") {
			return errors.New("schema app sudah ada tetapi marker tidak cocok; installer menolak menyentuh database")
		}
		if appSchemaExists {
			runtimeConn, err := pgx.Connect(ctx, runtimeURL.String())
			if err != nil {
				return errors.New("schema Algentra sudah terpasang, tetapi DATABASE_URL runtime tidak dapat login")
			}
			_ = runtimeConn.Close(ctx)
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Validated bool `json:"validated"`
		}{Validated: true})
	}
	if appSchemaExists {
		if projectMarker.Valid && projectMarker.String == "algentra:production:"+projectRef && schemaMarker.String == "algentra:revision:0005_equity_candle_indexes" {
			runtimeConn, err := pgx.Connect(ctx, runtimeURL.String())
			if err != nil {
				return errors.New("schema Algentra sudah terpasang, tetapi DATABASE_URL runtime tidak dapat login")
			}
			_ = runtimeConn.Close(ctx)
			return emitDatabaseURL(runtimeURL.String())
		}
		return errors.New("schema app sudah ada tetapi marker tidak cocok; installer menolak menyentuh database")
	}
	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("schema production gagal dipasang; database harus baru dan kosong: %w", err)
	}
	return emitDatabaseURL(runtimeURL.String())
}

func emitDatabaseURL(databaseURL string) error {
	result := struct {
		DatabaseURL string `json:"database_url"`
	}{DatabaseURL: databaseURL}
	return json.NewEncoder(os.Stdout).Encode(result)
}
