package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const keepBackups = 14

var sqliteBackupName = regexp.MustCompile(`^algentra-app-\d{8}T\d{6}Z\.db$`)
var postgresBackupName = regexp.MustCompile(`^algentra-postgres-\d{8}T\d{6}Z\.dump\.age$`)

func main() {
	if err := createBackup(); err != nil {
		fmt.Fprintln(os.Stderr, "backup gagal:", err)
		os.Exit(1)
	}
}

func createBackup() error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	if resolved, e := filepath.EvalSymlinks(root); e == nil {
		root = resolved
	}
	backupValue := strings.TrimSpace(os.Getenv("BACKUP_DIR"))
	if backupValue == "" {
		return errors.New("BACKUP_DIR wajib diarahkan ke lokasi backup persisten")
	}
	backupDir, err := filepath.Abs(expandHome(backupValue))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(backupDir, 0o700); err != nil {
		return fmt.Errorf("folder backup tidak dapat dibuat: %w", err)
	}
	if resolved, e := filepath.EvalSymlinks(backupDir); e == nil {
		backupDir = resolved
	}
	if withinPath(root, backupDir) {
		return errors.New("BACKUP_DIR harus berada di luar folder aplikasi")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL wajib diisi")
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") {
		runtime, err := parsePostgresURL(databaseURL)
		if err != nil {
			return fmt.Errorf("DATABASE_URL tidak valid: %w", err)
		}
		if strings.ToLower(strings.SplitN(runtime.User.Username(), ".", 2)[0]) != "algentra_api" {
			return errors.New("DATABASE_URL harus memakai role runtime algentra_api")
		}
		projectRef := strings.TrimSpace(os.Getenv("SUPABASE_PROJECT_REF"))
		if projectRef == "" {
			return errors.New("SUPABASE_PROJECT_REF wajib diisi di production untuk memeriksa target backup")
		}
		identity := strings.ToLower(runtime.Hostname() + " " + runtime.User.Username())
		if !strings.Contains(identity, strings.ToLower(projectRef)) {
			return errors.New("DATABASE_URL harus menunjuk project Supabase yang cocok dengan SUPABASE_PROJECT_REF")
		}
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	switch strings.ToLower(mustURLScheme(databaseURL)) {
	case "sqlite":
		destination := filepath.Join(backupDir, "algentra-app-"+stamp+".db")
		if err = createSQLiteBackup(databaseURL, backupDir, destination); err != nil {
			return err
		}
		if err = rotateBackups(backupDir, sqliteBackupName); err != nil {
			return err
		}
		fmt.Println(destination)
		return nil
	case "postgres", "postgresql", "postgresql+psycopg":
		destination := filepath.Join(backupDir, "algentra-postgres-"+stamp+".dump.age")
		if err = createPostgresBackup(databaseURL, destination); err != nil {
			return err
		}
		if err = rotateBackups(backupDir, postgresBackupName); err != nil {
			return err
		}
		fmt.Println(destination)
		return nil
	default:
		return errors.New("DATABASE_URL harus memakai PostgreSQL Supabase atau SQLite development")
	}
}

func mustURLScheme(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return u.Scheme
}
func projectRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv("PROJECT_ROOT")); value != "" {
		return filepath.Abs(value)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	current := cwd
	for {
		if _, err = os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return filepath.Abs(current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return filepath.Abs(cwd)
}
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimLeft(path[1:], "/\\"))
		}
	}
	return path
}
func withinPath(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return true
	}
	return relative == "." || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func sqliteFilename(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	path := u.Path
	if strings.HasPrefix(path, "//") {
		path = path[1:]
	} else {
		path = strings.TrimPrefix(path, "/")
	}
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
		path = path[1:]
	}
	if !filepath.IsAbs(filepath.FromSlash(path)) {
		return "", errors.New("DATABASE_URL SQLite harus berupa jalur absolut")
	}
	return filepath.Clean(filepath.FromSlash(path)), nil
}
func createSQLiteBackup(databaseURL, backupDir, destination string) error {
	databasePath, err := sqliteFilename(databaseURL)
	if err != nil {
		return err
	}
	sourceInfo, err := os.Stat(databasePath)
	if err != nil || !sourceInfo.Mode().IsRegular() {
		return errors.New("Database SQLite belum ada; backup tidak dibuat")
	}
	if resolved, e := filepath.EvalSymlinks(databasePath); e == nil {
		databasePath = resolved
	}
	dbDir, err := filepath.Abs(filepath.Dir(databasePath))
	if err != nil {
		return err
	}
	if filepath.Clean(backupDir) == filepath.Clean(dbDir) || withinPath(backupDir, databasePath) {
		return errors.New("BACKUP_DIR harus berada di luar folder database")
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return errors.New("database SQLite tidak dapat dibuka")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil {
		return errors.New("database SQLite tidak tersedia")
	}
	quoted := strings.ReplaceAll(destination, "'", "''")
	if _, err = db.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		_ = os.Remove(destination)
		return errors.New("backup SQLite tidak dapat dibuat")
	}
	copyDB, err := sql.Open("sqlite", destination)
	if err != nil {
		_ = os.Remove(destination)
		return errors.New("backup SQLite tidak dapat diperiksa")
	}
	defer copyDB.Close()
	var integrity string
	if err = copyDB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		_ = os.Remove(destination)
		return errors.New("pemeriksaan integritas backup gagal")
	}
	return nil
}

func createPostgresBackup(databaseURL, destination string) error {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	backupURLValue := strings.TrimSpace(os.Getenv("BACKUP_DATABASE_URL"))
	if appEnv == "production" && backupURLValue == "" {
		return errors.New("BACKUP_DATABASE_URL wajib memakai role backup read-only di production")
	}
	if backupURLValue == "" {
		backupURLValue = databaseURL
	}
	backup, err := parsePostgresURL(backupURLValue)
	if err != nil {
		return fmt.Errorf("BACKUP_DATABASE_URL tidak valid: %w", err)
	}
	runtime, err := parsePostgresURL(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL tidak valid: %w", err)
	}
	if backup.User.Username() == runtime.User.Username() {
		return errors.New("BACKUP_DATABASE_URL harus memakai role read-only terpisah dari role runtime")
	}
	recipient := strings.TrimSpace(os.Getenv("BACKUP_AGE_RECIPIENT"))
	if recipient == "" {
		return errors.New("BACKUP_AGE_RECIPIENT wajib diisi agar pg_dump dienkripsi")
	}
	dumpFile, err := os.CreateTemp(filepath.Dir(destination), "algentra-postgres-*.dump")
	if err != nil {
		return err
	}
	temporary := dumpFile.Name()
	defer os.Remove(temporary)
	if err = dumpFile.Close(); err != nil {
		return err
	}
	port := backup.Port()
	if port == "" {
		port = "5432"
	}
	password, _ := backup.User.Password()
	dbName := strings.TrimPrefix(backup.Path, "/")
	env := filteredCommandEnvironment(os.Environ())
	env = append(env, "PGHOST="+backup.Hostname(), "PGPORT="+port, "PGUSER="+backup.User.Username(), "PGPASSWORD="+password, "PGDATABASE="+dbName, "PGSSLMODE="+backup.Query().Get("sslmode"), "PGCONNECT_TIMEOUT=10")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return errors.New("executable backup tidak ditemukan: pg_dump")
	}
	dump := exec.CommandContext(ctx, pgDump, "--no-owner", "--no-acl", "--format=custom", "--schema=app", "--file", temporary)
	dump.Env = env
	if _, err = dump.CombinedOutput(); err != nil {
		return errors.New("Backup Supabase gagal (pg_dump); periksa koneksi dan izin backup")
	}
	age, err := exec.LookPath("age")
	if err != nil {
		return errors.New("executable backup tidak ditemukan: age")
	}
	encrypt := exec.CommandContext(ctx, age, "--encrypt", "--recipient", recipient, "--output", destination, temporary)
	encrypt.Env = filteredCommandEnvironment(os.Environ())
	if _, err = encrypt.CombinedOutput(); err != nil {
		_ = os.Remove(destination)
		return errors.New("Backup Supabase gagal (age encryption)")
	}
	info, err := os.Stat(destination)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(destination)
		return errors.New("File backup Supabase terenkripsi kosong")
	}
	return nil
}

func filteredCommandEnvironment(source []string) []string {
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

func parsePostgresURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("URL tidak valid")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" && u.Scheme != "postgresql+psycopg" {
		return nil, errors.New("harus berupa connection string PostgreSQL")
	}
	if u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Path == "" || u.Path == "/" {
		return nil, errors.New("connection string PostgreSQL harus lengkap")
	}
	if _, ok := u.User.Password(); !ok {
		return nil, errors.New("connection string PostgreSQL harus berisi kredensial")
	}
	ssl := strings.ToLower(u.Query().Get("sslmode"))
	if ssl != "require" && ssl != "verify-ca" && ssl != "verify-full" {
		return nil, errors.New("connection string PostgreSQL harus mewajibkan TLS")
	}
	return u, nil
}

func rotateBackups(directory string, pattern *regexp.Regexp) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && pattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[keepBackups:] {
		if err = os.Remove(filepath.Join(directory, name)); err != nil {
			return fmt.Errorf("backup lama tidak dapat dirotasi: %w", err)
		}
	}
	return nil
}
