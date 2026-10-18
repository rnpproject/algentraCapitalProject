// import-sqlite-to-supabase copies a local SQLite database into a marked
// Supabase development project. It never updates existing target rows.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const expectedRevision = "0004_account_daily_profit"

type column struct {
	name     string
	typeName string
	primary  bool
	pkOrder  int
}

type table struct {
	name    string
	columns []column
}

func main() {
	dryRun := flag.Bool("dry-run", false, "validasi project dan checksum tanpa menulis data")
	flag.Parse()
	if flag.NArg() > 1 {
		fatal(errors.New("penggunaan: import-sqlite-to-supabase [--dry-run] [source.db]"))
	}
	root, err := projectRoot()
	if err != nil {
		fatal(err)
	}
	source := filepath.Join(root, "backend", "data", "app.db")
	if flag.NArg() == 1 {
		source = flag.Arg(0)
		if !filepath.IsAbs(source) {
			source = filepath.Join(root, source)
		}
	}
	if err := run(source, *dryRun, root); err != nil {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "impor gagal:", err); os.Exit(1) }

func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", errors.New("root project Go tidak ditemukan")
}

func readToolConfig(root string) (map[string]string, error) {
	values := map[string]string{}
	file, err := os.Open(filepath.Join(root, ".env.local"))
	if err != nil {
		return nil, errors.New("file .env.local development tidak ditemukan")
	}
	defer file.Close()
	// The environment file is intentionally read without logging any values.
	data, err := io.ReadAll(io.LimitReader(file, 1<<20))
	if err != nil {
		return nil, err
	}
	for _, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		switch name {
		case "APP_ENV", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SUPABASE_DEV_PROJECT_REF":
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
	return values, nil
}

func validateURL(raw, role string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql" && u.Scheme != "postgresql+psycopg") || u.Hostname() == "" || u.User == nil || u.Path == "" || u.Path == "/" {
		return nil, fmt.Errorf("URL %s harus berupa connection string PostgreSQL lengkap", role)
	}
	if _, ok := u.User.Password(); !ok {
		return nil, fmt.Errorf("URL %s harus berisi kredensial", role)
	}
	ssl := strings.ToLower(u.Query().Get("sslmode"))
	if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
		return nil, fmt.Errorf("URL %s harus mewajibkan TLS", role)
	}
	username := strings.ToLower(strings.SplitN(u.User.Username(), ".", 2)[0])
	if username != role {
		return nil, fmt.Errorf("URL database harus memakai role %s", role)
	}
	if u.Scheme == "postgresql+psycopg" {
		u.Scheme = "postgresql"
	}
	return u, nil
}

func run(sourcePath string, dry bool, root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	values, err := readToolConfig(root)
	if err != nil {
		return err
	}
	processEnvironment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if processEnvironment == "" {
		processEnvironment = "development"
	}
	if !strings.EqualFold(processEnvironment, "development") || !strings.EqualFold(values["APP_ENV"], "development") {
		return errors.New("operasi ini hanya boleh dijalankan saat APP_ENV=development pada environment dan .env.local")
	}
	ref := strings.TrimSpace(values["SUPABASE_DEV_PROJECT_REF"])
	if ref == "" {
		return errors.New("SUPABASE_DEV_PROJECT_REF wajib diisi")
	}
	runtimeURL, err := validateURL(values["DATABASE_URL"], "algentra_api")
	if err != nil {
		return err
	}
	migratorURL, err := validateURL(values["MIGRATION_DATABASE_URL"], "algentra_migrator")
	if err != nil {
		return err
	}
	if runtimeURL.Path != migratorURL.Path || !strings.Contains(strings.ToLower(runtimeURL.Host+" "+runtimeURL.User.Username()), strings.ToLower(ref)) || !strings.Contains(strings.ToLower(migratorURL.Host+" "+migratorURL.User.Username()), strings.ToLower(ref)) {
		return errors.New("DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama")
	}
	info, err := os.Stat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("file SQLite tidak ditemukan: %s", sourcePath)
	}
	source, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return errors.New("database SQLite tidak dapat dibuka")
	}
	defer source.Close()
	source.SetMaxOpenConns(1)
	if err = source.PingContext(ctx); err != nil {
		return errors.New("database SQLite tidak tersedia")
	}
	validation, err := sql.Open("pgx", migratorURL.String())
	if err != nil {
		return errors.New("koneksi validasi database tidak dapat dibuat")
	}
	defer validation.Close()
	if err = validateTarget(ctx, validation, ref); err != nil {
		return err
	}
	target, err := sql.Open("pgx", runtimeURL.String())
	if err != nil {
		return errors.New("koneksi database target tidak dapat dibuat")
	}
	defer target.Close()
	target.SetMaxOpenConns(1)
	if err = target.PingContext(ctx); err != nil {
		return errors.New("database target tidak tersedia")
	}
	sourceTables, err := sqliteTables(ctx, source)
	if err != nil {
		return err
	}
	targetTables, err := postgresTables(ctx, target)
	if err != nil {
		return err
	}
	for i := range sourceTables {
		if sourceTables[i].name == "mt5_accounts" {
			sourceTables[i].columns = without(sourceTables[i].columns, "token_ciphertext")
		}
	}
	if !sameSchema(sourceTables, targetTables) {
		return errors.New("skema SQLite dan schema app Supabase tidak sama; selesaikan pemetaan versi skema sebelum impor")
	}
	// Keep the transaction open across checksum verification and inserts so a
	// mismatch rolls back every table imported in this run.
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET search_path TO app"); err != nil {
		return errors.New("schema app tidak dapat dipilih")
	}
	type result struct {
		name   string
		count  int64
		action string
	}
	results := make([]result, 0, len(sourceTables))
	for _, t := range sourceTables {
		sourceCount, sourceHash, err := checksum(ctx, source, t)
		if err != nil {
			return fmt.Errorf("checksum SQLite %s gagal: %w", t.name, err)
		}
		targetCount, targetHash, err := checksum(ctx, tx, t)
		if err != nil {
			return fmt.Errorf("checksum Supabase %s gagal: %w", t.name, err)
		}
		action := "already identical"
		if sourceCount != targetCount || sourceHash != targetHash {
			if dry {
				action = fmt.Sprintf("would import/verify (%d existing, %d source)", targetCount, sourceCount)
			} else {
				if err = importTable(ctx, source, tx, t); err != nil {
					return fmt.Errorf("impor %s gagal: %w", t.name, err)
				}
				count, digest, e := checksum(ctx, tx, t)
				if e != nil {
					return e
				}
				if count != sourceCount || digest != sourceHash {
					return fmt.Errorf("checksum berbeda setelah impor %s; transaksi dibatalkan", t.name)
				}
				action = "imported and verified"
			}
		}
		results = append(results, result{t.name, sourceCount, action})
	}
	if !dry {
		for _, t := range sourceTables {
			if err = syncSequence(ctx, tx, t); err != nil {
				return fmt.Errorf("sinkronisasi sequence %s gagal: %w", t.name, err)
			}
		}
		if err = tx.Commit(); err != nil {
			return errors.New("transaksi impor tidak dapat disimpan")
		}
	}
	var total int64
	for _, r := range results {
		total += r.count
	}
	title := "Imported/verified"
	if dry {
		title = "Dry-run checked"
	}
	fmt.Printf("%s %d source rows across %d tables in Supabase development:\n", title, total, len(results))
	for _, r := range results {
		fmt.Printf("  %s: %d source rows; %s\n", r.name, r.count, r.action)
	}
	return nil
}

func validateTarget(ctx context.Context, db *sql.DB, ref string) error {
	var marker, revision, schemaMarker sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname=current_database()").Scan(&marker); err != nil {
		return errors.New("marker project tidak dapat dibaca menggunakan role migrator")
	}
	if marker.String != "algentra:development:"+ref {
		return errors.New("impor ditolak: target bukan project development yang ditandai dengan benar")
	}
	if err := db.QueryRowContext(ctx, "SELECT version_num FROM public.alembic_version").Scan(&revision); err != nil || revision.String != expectedRevision {
		return fmt.Errorf("impor ditolak: jalankan migrasi development terlebih dahulu (expected=%s)", expectedRevision)
	}
	if err := db.QueryRowContext(ctx, "SELECT obj_description('app'::regnamespace, 'pg_namespace')").Scan(&schemaMarker); err != nil || schemaMarker.String != "algentra:revision:"+expectedRevision {
		return errors.New("impor ditolak: comment revision schema app tidak cocok dengan Alembic")
	}
	return nil
}

func sqliteTables(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []table
	for rows.Next() {
		var t table
		if err = rows.Scan(&t.name); err != nil {
			return nil, err
		}
		cols, e := db.QueryContext(ctx, "PRAGMA table_info("+quoteSQLite(t.name)+")")
		if e != nil {
			return nil, e
		}
		for cols.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt any
			if e = cols.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); e != nil {
				cols.Close()
				return nil, e
			}
			t.columns = append(t.columns, column{name: name, typeName: strings.ToUpper(typ), primary: pk > 0, pkOrder: pk})
		}
		e = cols.Err()
		cols.Close()
		if e != nil {
			return nil, e
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func postgresTables(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(ctx, `SELECT c.table_name,c.column_name,c.data_type,COALESCE((SELECT ku.ordinal_position FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage ku USING (constraint_catalog,constraint_schema,constraint_name,table_name) WHERE tc.constraint_type='PRIMARY KEY' AND tc.table_schema=c.table_schema AND tc.table_name=c.table_name AND ku.column_name=c.column_name),0) AS pk_order FROM information_schema.columns c WHERE c.table_schema='app' ORDER BY c.table_name,c.ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []table
	for rows.Next() {
		var tn, cn, typ string
		var pkOrder int
		if err = rows.Scan(&tn, &cn, &typ, &pkOrder); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].name != tn {
			result = append(result, table{name: tn})
		}
		result[len(result)-1].columns = append(result[len(result)-1].columns, column{name: cn, typeName: strings.ToUpper(typ), primary: pkOrder > 0, pkOrder: pkOrder})
	}
	return result, rows.Err()
}

func sameSchema(a, b []table) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || len(a[i].columns) != len(b[i].columns) {
			return false
		}
		for j := range a[i].columns {
			if a[i].columns[j].name != b[i].columns[j].name { // redacted legacy ciphertext is the only permitted extra source column
				if a[i].columns[j].name == "token_ciphertext" {
					continue
				}
				return false
			}
			if a[i].columns[j].primary != b[i].columns[j].primary {
				return false
			}
		}
	}
	return true
}

func without(cols []column, name string) []column {
	out := make([]column, 0, len(cols))
	for _, c := range cols {
		if c.name != name {
			out = append(out, c)
		}
	}
	return out
}
func quoteSQLite(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func quotePG(s string) string     { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func orderedColumns(t table) ([]column, []column) {
	cols := append([]column(nil), t.columns...)
	pk := make([]column, 0)
	for _, c := range cols {
		if c.primary {
			pk = append(pk, c)
		}
	}
	sort.SliceStable(pk, func(i, j int) bool { return pk[i].pkOrder < pk[j].pkOrder })
	if len(pk) == 0 {
		pk = cols
	}
	return cols, pk
}
func selectSQL(t table) string {
	cols, order := orderedColumns(t)
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quotePG(c.name)
	}
	keys := make([]string, len(order))
	for i, c := range order {
		keys[i] = quotePG(c.name)
	}
	return "SELECT " + strings.Join(names, ",") + " FROM " + quotePG(t.name) + " ORDER BY " + strings.Join(keys, ",")
}

func checksum(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, t table) (int64, string, error) {
	rows, err := q.QueryContext(ctx, selectSQL(t))
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	cols, _ := orderedColumns(t)
	digest := sha256.New()
	var count int64
	for rows.Next() {
		vals := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return 0, "", err
		}
		canonical := make([]any, len(vals))
		for i, v := range vals {
			canonical[i] = canonicalValue(v, cols[i].typeName)
		}
		data, e := json.Marshal(canonical)
		if e != nil {
			return 0, "", e
		}
		digest.Write(data)
		digest.Write([]byte{'\n'})
		count++
	}
	if err = rows.Err(); err != nil {
		return 0, "", err
	}
	return count, hex.EncodeToString(digest.Sum(nil)), nil
}

func canonicalValue(v any, typ string) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		if strings.Contains(typ, "JSON") {
			var parsed any
			if json.Unmarshal(x, &parsed) == nil {
				return parsed
			}
		}
		if strings.Contains(typ, "DATE") || strings.Contains(typ, "TIME") {
			if value, ok := parseTimestamp(string(x)); ok {
				return value
			}
		}
		return map[string]string{"bytes_hex": hex.EncodeToString(x)}
	case time.Time:
		return x.UTC().Format("2006-01-02T15:04:05.000000Z")
	case int64:
		if strings.Contains(typ, "BOOL") {
			return x != 0
		}
		return x
	case float64:
		return x
	case string:
		if strings.Contains(typ, "BOOL") {
			return strings.EqualFold(x, "true") || x == "1"
		}
		if strings.Contains(typ, "JSON") {
			var parsed any
			if json.Unmarshal([]byte(x), &parsed) == nil {
				return parsed
			}
		}
		if strings.Contains(typ, "DATE") || strings.Contains(typ, "TIME") {
			if value, ok := parseTimestamp(x); ok {
				return value
			}
		}
		return x
	default:
		return x
	}
}

func parseTimestamp(s string) (string, bool) {
	layouts := []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, e := time.Parse(layout, s); e == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000000Z"), true
		}
	}
	return "", false
}

func importTable(ctx context.Context, source *sql.DB, target *sql.Tx, t table) error {
	cols, _ := orderedColumns(t)
	names := make([]string, len(cols))
	marks := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quotePG(c.name)
		marks[i] = fmt.Sprintf("$%d", i+1)
	}
	query := strings.Replace(selectSQL(t), " FROM ", " FROM ", 1)
	rows, err := source.QueryContext(ctx, strings.ReplaceAll(query, quotePG(t.name), quoteSQLite(t.name)))
	if err != nil {
		return err
	}
	defer rows.Close()
	insert := "INSERT INTO " + quotePG(t.name) + " (" + strings.Join(names, ",") + ") VALUES (" + strings.Join(marks, ",") + ") ON CONFLICT DO NOTHING"
	for rows.Next() {
		values := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			return err
		}
		for i, c := range cols {
			if strings.Contains(c.typeName, "BOOL") && values[i] != nil {
				switch b := values[i].(type) {
				case int64:
					values[i] = b != 0
				case string:
					values[i] = b == "1" || strings.EqualFold(b, "true")
				}
			}
		}
		if _, err = target.ExecContext(ctx, insert, values...); err != nil {
			return err
		}
	}
	return rows.Err()
}

func syncSequence(ctx context.Context, tx *sql.Tx, t table) error {
	cols, _ := orderedColumns(t)
	var pk []column
	for _, c := range cols {
		if c.primary {
			pk = append(pk, c)
		}
	}
	if len(pk) != 1 {
		return nil
	}
	c := pk[0]
	if !strings.Contains(c.typeName, "INT") {
		return nil
	}
	var seq sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT pg_get_serial_sequence($1,$2)", "app."+t.name, c.name).Scan(&seq); err != nil || !seq.Valid {
		return err
	}
	var max sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT MAX("+quotePG(c.name)+") FROM "+quotePG(t.name)).Scan(&max); err != nil {
		return err
	}
	if !max.Valid {
		return nil
	}
	_, err := tx.ExecContext(ctx, "SELECT setval($1::regclass,$2,TRUE)", seq.String, max.Int64)
	return err
}
