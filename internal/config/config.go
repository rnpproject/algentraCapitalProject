package config

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Settings struct {
	AppEnv                  string
	AppSecretKey            string
	AdminUsername           string
	AdminPassword           string
	CookieSecure            bool
	DatabaseURL             string
	SupabaseDevRef          string
	SupabaseProjectRef      string
	BFFHMACSecret           string
	EAAPIKey                string
	CopierWorkerAPIKey      string
	HTTPAddr                string
	WebSocketOrigins        []string
	ContactPersonName       string
	ContactWhatsApp         string
	ContactEmail            string
	ContactInstagram        string
	ContactTelegram         string
	ContactTelegramAdmin    string
	ContactTikTok           string
	ContactWebsite          string
	ContactFacebook         string
	ContactLinkedIn         string
	ContactX                string
	ContactYouTube          string
	PublicMusicURL          string
	GeminiAPIKey            string
	GeminiModel             string
	GeminiTimeoutSeconds    float64
	GeminiRetryAttempts     int
	EnableRegexFallback     bool
	GeminiCSAPIKey          string
	GeminiCSModel           string
	GeminiCSTimeoutSeconds  int
	PaymentBankName         string
	PaymentNumber           string
	PaymentName             string
	MidtransConfigured      bool
	MidtransServerKey       string
	MidtransMerchantID      string
	MidtransProduction      bool
	MidtransNotificationURL string
	TelegramBotToken        string
	TelegramAdminChatID     string
	TelegramAPIID           int
	TelegramAPIHash         string
	EmailSMTPHost           string
	EmailSMTPPort           int
	EmailSMTPUsername       string
	EmailSMTPPassword       string
	EmailFrom               string
	EmailSMTPStartTLS       bool
	EmailConfigured         bool
	ManagementFeeIDR        int64
	VPSOptions              []VPSOption
}

type VPSOption struct {
	Code     string
	Name     string
	VCPU     string
	RAM      string
	Disk     string
	PriceIDR int64
}

func Load() (Settings, error) {
	if strings.ToLower(valueOr(os.Getenv("APP_ENV"), "development")) == "development" {
		path := os.Getenv("ALGENTRA_ENV_FILE")
		if path == "" {
			path = filepath.Join(".env.local")
		}
		if err := loadEnvFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Settings{}, fmt.Errorf("baca file environment: %w", err)
		}
	}
	s := Settings{
		AppEnv:                  strings.ToLower(valueOr(os.Getenv("APP_ENV"), "development")),
		AppSecretKey:            os.Getenv("APP_SECRET_KEY"),
		AdminUsername:           valueOr(os.Getenv("ADMIN_USERNAME"), "admin"),
		AdminPassword:           os.Getenv("ADMIN_PASSWORD"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		SupabaseDevRef:          strings.TrimSpace(os.Getenv("SUPABASE_DEV_PROJECT_REF")),
		SupabaseProjectRef:      strings.TrimSpace(os.Getenv("SUPABASE_PROJECT_REF")),
		BFFHMACSecret:           os.Getenv("BFF_HMAC_SECRET"),
		EAAPIKey:                os.Getenv("EA_API_KEY"),
		CopierWorkerAPIKey:      os.Getenv("COPIER_WORKER_API_KEY"),
		HTTPAddr:                valueOr(os.Getenv("HTTP_ADDR"), "127.0.0.1:8000"),
		ContactPersonName:       strings.TrimSpace(os.Getenv("CONTACT_PERSON_NAME")),
		ContactWhatsApp:         strings.TrimSpace(os.Getenv("CONTACT_WHATSAPP")),
		ContactEmail:            strings.TrimSpace(os.Getenv("CONTACT_EMAIL")),
		ContactInstagram:        strings.TrimSpace(os.Getenv("CONTACT_INSTAGRAM")),
		ContactTelegram:         strings.TrimSpace(os.Getenv("CONTACT_TELEGRAM")),
		ContactTelegramAdmin:    strings.TrimSpace(os.Getenv("CONTACT_TELEGRAM_ADMIN")),
		ContactTikTok:           strings.TrimSpace(os.Getenv("CONTACT_TIKTOK")),
		ContactWebsite:          strings.TrimSpace(os.Getenv("CONTACT_WEBSITE")),
		ContactFacebook:         strings.TrimSpace(os.Getenv("CONTACT_FACEBOOK")),
		ContactLinkedIn:         strings.TrimSpace(os.Getenv("CONTACT_LINKEDIN")),
		ContactX:                strings.TrimSpace(os.Getenv("CONTACT_X")),
		ContactYouTube:          strings.TrimSpace(os.Getenv("CONTACT_YOUTUBE")),
		PublicMusicURL:          strings.TrimSpace(os.Getenv("PUBLIC_MUSIC_URL")),
		GeminiAPIKey:            strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:             valueOr(os.Getenv("GEMINI_MODEL"), "gemini-3.8-flash"),
		GeminiTimeoutSeconds:    25,
		GeminiRetryAttempts:     3,
		GeminiCSAPIKey:          strings.TrimSpace(os.Getenv("GEMINI_CS_API_KEY")),
		GeminiCSModel:           valueOr(os.Getenv("GEMINI_CS_MODEL"), "gemini-3.8-flash"),
		GeminiCSTimeoutSeconds:  60,
		PaymentBankName:         strings.TrimSpace(os.Getenv("PAYMENT_BANK_NAME")),
		PaymentNumber:           strings.TrimSpace(os.Getenv("PAYMENT_NUMBER")),
		PaymentName:             strings.TrimSpace(os.Getenv("PAYMENT_NAME")),
		MidtransServerKey:       strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY")),
		MidtransMerchantID:      strings.TrimSpace(os.Getenv("MIDTRANS_MERCHANT_ID")),
		MidtransNotificationURL: strings.TrimSpace(os.Getenv("MIDTRANS_NOTIFICATION_URL")),
		TelegramBotToken:        strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramAdminChatID:     strings.TrimSpace(os.Getenv("TELEGRAM_ADMIN_CHAT_ID")),
		EmailSMTPHost:           strings.TrimSpace(os.Getenv("EMAIL_SMTP_HOST")),
		EmailSMTPUsername:       strings.TrimSpace(os.Getenv("EMAIL_SMTP_USERNAME")),
		EmailSMTPPassword:       os.Getenv("EMAIL_SMTP_PASSWORD"),
		EmailFrom:               strings.TrimSpace(os.Getenv("EMAIL_FROM")),
	}
	var err error
	s.GeminiTimeoutSeconds, err = floatEnv("GEMINI_TIMEOUT_SECONDS", 25)
	if err != nil || math.IsNaN(s.GeminiTimeoutSeconds) || math.IsInf(s.GeminiTimeoutSeconds, 0) || s.GeminiTimeoutSeconds <= 0 || s.GeminiTimeoutSeconds > 120 {
		return Settings{}, errors.New("GEMINI_TIMEOUT_SECONDS harus lebih besar dari 0 dan maksimal 120")
	}
	s.GeminiRetryAttempts, err = intEnv("GEMINI_RETRY_ATTEMPTS", 3)
	if err != nil || s.GeminiRetryAttempts < 1 || s.GeminiRetryAttempts > 6 {
		return Settings{}, errors.New("GEMINI_RETRY_ATTEMPTS harus antara 1 dan 6")
	}
	s.EnableRegexFallback, err = boolEnv("ENABLE_REGEX_FALLBACK", false)
	if err != nil {
		return Settings{}, err
	}
	s.CookieSecure, err = boolEnv("COOKIE_SECURE", true)
	if err != nil {
		return Settings{}, err
	}
	s.WebSocketOrigins, err = normalizeOrigins(os.Getenv("WEBSOCKET_ALLOWED_ORIGINS"))
	if err != nil {
		return Settings{}, err
	}
	s.MidtransProduction, err = boolEnv("MIDTRANS_IS_PRODUCTION", false)
	if err != nil {
		return Settings{}, err
	}
	s.MidtransConfigured = s.MidtransServerKey != ""
	s.EmailSMTPPort, err = intEnv("EMAIL_SMTP_PORT", 587)
	if err != nil || s.EmailSMTPPort < 1 || s.EmailSMTPPort > 65535 {
		return Settings{}, errors.New("EMAIL_SMTP_PORT harus antara 1 dan 65535")
	}
	s.EmailSMTPStartTLS, err = boolEnv("EMAIL_SMTP_STARTTLS", true)
	if err != nil {
		return Settings{}, err
	}
	s.GeminiCSTimeoutSeconds, err = intEnv("GEMINI_CS_TIMEOUT_SECONDS", 60)
	if err != nil || s.GeminiCSTimeoutSeconds < 15 || s.GeminiCSTimeoutSeconds > 180 {
		return Settings{}, errors.New("GEMINI_CS_TIMEOUT_SECONDS harus antara 15 dan 180")
	}
	configuredEmail := 0
	for _, value := range []string{s.EmailSMTPHost, s.EmailSMTPUsername, s.EmailSMTPPassword, s.EmailFrom} {
		if value != "" {
			configuredEmail++
		}
	}
	if configuredEmail != 0 && configuredEmail != 4 {
		return Settings{}, errors.New("EMAIL_SMTP_HOST, EMAIL_SMTP_USERNAME, EMAIL_SMTP_PASSWORD, dan EMAIL_FROM harus diisi bersama")
	}
	s.EmailConfigured = configuredEmail == 4
	s.TelegramAPIID, err = intEnv("TELEGRAM_API_ID", 0)
	if err != nil {
		return Settings{}, err
	}
	s.TelegramAPIHash = strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH"))
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	if err := loadServicePricing(&s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func loadServicePricing(s *Settings) error {
	fee, err := int64Env("MANAGEMENT_FEE_IDR", 150000)
	if err != nil || fee < 0 {
		return errors.New("MANAGEMENT_FEE_IDR harus integer nonnegatif")
	}
	s.ManagementFeeIDR = fee
	for i := 1; i <= 5; i++ {
		prefix := fmt.Sprintf("VPS%d_", i)
		name := strings.TrimSpace(os.Getenv(prefix + "NAME"))
		if len([]rune(name)) > 80 {
			return fmt.Errorf("%sNAME maksimal 80 karakter", prefix)
		}
		if name == "" {
			continue
		}
		vcpu, ram, disk := strings.TrimSpace(os.Getenv(prefix+"VCPU")), strings.TrimSpace(os.Getenv(prefix+"RAM")), strings.TrimSpace(os.Getenv(prefix+"DISK"))
		if len([]rune(vcpu)) > 32 || len([]rune(ram)) > 32 || len([]rune(disk)) > 32 {
			return fmt.Errorf("spesifikasi %s melebihi 32 karakter", strings.TrimSuffix(prefix, "_"))
		}
		price, err := int64Env(prefix+"PRICE_IDR", 0)
		if err != nil || price < 0 {
			return fmt.Errorf("%sPRICE_IDR harus integer nonnegatif", prefix)
		}
		if vcpu == "" {
			vcpu = "—"
		}
		if ram == "" {
			ram = "—"
		}
		if disk == "" {
			disk = "—"
		}
		s.VPSOptions = append(s.VPSOptions, VPSOption{Code: fmt.Sprintf("VPS%d", i), Name: name, VCPU: vcpu, RAM: ram, Disk: disk, PriceIDR: price})
	}
	return nil
}

func int64Env(key string, fallback int64) (int64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err
}

func intEnv(key string, fallback int) (int, error) {
	v, err := int64Env(key, int64(fallback))
	if err != nil {
		return 0, err
	}
	if v > int64(^uint(0)>>1) || v < 0 {
		return 0, errors.New(key + " tidak valid")
	}
	return int(v), nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err
}

func (s Settings) Validate() error {
	if s.AppEnv != "production" && s.AppEnv != "development" && s.AppEnv != "test" {
		return errors.New("APP_ENV harus production, development, atau test")
	}
	if s.AppEnv == "production" && strings.ToLower(os.Getenv("APP_ENV")) != "production" {
		return errors.New("APP_ENV=production harus disetel pada environment proses")
	}
	if len(s.AppSecretKey) < 32 || hasPlaceholder(s.AppSecretKey, "replace-with", "change-this", "changethis", "your-secret") {
		return errors.New("APP_SECRET_KEY harus berupa secret acak minimal 32 karakter")
	}
	if len(s.AdminPassword) < 12 || hasPlaceholder(s.AdminPassword, "replace-with", "change-this", "changethis", "your-password") {
		return errors.New("ADMIN_PASSWORD harus berupa password unik minimal 12 karakter")
	}
	if strings.TrimSpace(s.AdminUsername) == "" {
		return errors.New("ADMIN_USERNAME tidak boleh kosong")
	}
	if utf8.RuneCountInString(s.PaymentBankName) > 120 {
		return errors.New("PAYMENT_BANK_NAME maksimal 120 karakter")
	}
	if utf8.RuneCountInString(s.PaymentNumber) > 80 {
		return errors.New("PAYMENT_NUMBER maksimal 80 karakter")
	}
	if utf8.RuneCountInString(s.PaymentName) > 120 {
		return errors.New("PAYMENT_NAME maksimal 120 karakter")
	}
	if utf8.RuneCountInString(s.MidtransNotificationURL) > 320 {
		return errors.New("MIDTRANS_NOTIFICATION_URL maksimal 320 karakter")
	}
	if (s.TelegramBotToken == "") != (s.TelegramAdminChatID == "") {
		return errors.New("TELEGRAM_BOT_TOKEN dan TELEGRAM_ADMIN_CHAT_ID harus diisi bersamaan")
	}
	if (s.TelegramAPIID == 0) != (s.TelegramAPIHash == "") {
		return errors.New("TELEGRAM_API_ID dan TELEGRAM_API_HASH harus diisi bersamaan")
	}
	if s.TelegramAPIID < 0 || (s.TelegramAPIID > 0 && !validTelegramAPIHash(s.TelegramAPIHash)) {
		return errors.New("TELEGRAM_API_ID harus positif dan TELEGRAM_API_HASH valid")
	}
	if s.TelegramAdminChatID != "" {
		validDigits := true
		for _, digit := range s.TelegramAdminChatID {
			if digit < '0' || digit > '9' {
				validDigits = false
				break
			}
		}
		if !validDigits {
			return errors.New("TELEGRAM_ADMIN_CHAT_ID harus berupa ID chat pribadi admin yang positif")
		}
		if id, err := strconv.ParseInt(s.TelegramAdminChatID, 10, 64); err != nil || id < 1 {
			return errors.New("TELEGRAM_ADMIN_CHAT_ID harus berupa ID chat pribadi admin yang positif")
		}
	}
	if s.AppEnv == "production" {
		if !s.CookieSecure {
			return errors.New("COOKIE_SECURE tidak boleh false di production")
		}
		if s.DatabaseURL == "" || s.SupabaseProjectRef == "" || len(s.BFFHMACSecret) < 32 {
			return errors.New("DATABASE_URL, SUPABASE_PROJECT_REF, dan BFF_HMAC_SECRET wajib di production")
		}
		if os.Getenv("MIGRATION_DATABASE_URL") != "" {
			return errors.New("MIGRATION_DATABASE_URL tidak boleh dipasang pada service production")
		}
	}
	if migrationURL, present := os.LookupEnv("MIGRATION_DATABASE_URL"); present && migrationURL != "" {
		if err := validateMigrationDatabaseURL(s, migrationURL); err != nil {
			return err
		}
	}
	if s.BFFHMACSecret != "" && len(s.BFFHMACSecret) < 32 {
		return errors.New("BFF_HMAC_SECRET minimal 32 karakter")
	}
	if s.PublicMusicURL != "" {
		musicURL, err := url.Parse(s.PublicMusicURL)
		if err != nil || !strings.EqualFold(musicURL.Scheme, "https") || musicURL.Hostname() == "" || musicURL.User != nil {
			return errors.New("PUBLIC_MUSIC_URL harus berupa URL HTTPS yang valid")
		}
		if rawPort := musicURL.Port(); rawPort != "" {
			port, err := strconv.Atoi(rawPort)
			if err != nil || port < 0 || port > 65535 {
				return errors.New("PUBLIC_MUSIC_URL harus berupa URL HTTPS yang valid")
			}
		}
	}
	if s.EAAPIKey != "" && (len(s.EAAPIKey) < 24 || hasPlaceholder(s.EAAPIKey, "replace-with", "change-this", "changethis", "your-api-key")) {
		return errors.New("EA_API_KEY harus berupa API key non-placeholder minimal 24 karakter")
	}
	if s.CopierWorkerAPIKey != "" && (len(s.CopierWorkerAPIKey) < 32 || hasPlaceholder(s.CopierWorkerAPIKey, "replace-with", "change-this", "your-api-key")) {
		return errors.New("COPIER_WORKER_API_KEY harus berupa secret non-placeholder minimal 32 karakter")
	}
	if s.DatabaseURL != "" {
		if err := validateDatabaseURL(s); err != nil {
			return err
		}
	}
	return nil
}

func validTelegramAPIHash(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateDatabaseURL(s Settings) error {
	u, err := url.Parse(s.DatabaseURL)
	if err != nil || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || strings.Trim(u.Path, "/") == "" {
		return errors.New("DATABASE_URL harus berupa connection string PostgreSQL lengkap")
	}
	password, hasPassword := u.User.Password()
	if !hasPassword || password == "" {
		return errors.New("DATABASE_URL harus menyertakan password role runtime")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "postgres" && scheme != "postgresql" && scheme != "postgresql+psycopg" {
		return errors.New("DATABASE_URL harus memakai PostgreSQL Supabase")
	}
	username := strings.ToLower(strings.SplitN(u.User.Username(), ".", 2)[0])
	if username != "algentra_api" {
		return errors.New("DATABASE_URL harus memakai role runtime algentra_api")
	}
	sslmode := strings.ToLower(u.Query().Get("sslmode"))
	if sslmode != "require" && sslmode != "verify-ca" && sslmode != "verify-full" {
		return errors.New("DATABASE_URL Supabase harus mewajibkan TLS lewat sslmode")
	}
	expectedRef := ""
	switch s.AppEnv {
	case "development":
		expectedRef = s.SupabaseDevRef
	case "production":
		expectedRef = s.SupabaseProjectRef
	}
	if s.AppEnv == "development" && expectedRef == "" {
		return errors.New("SUPABASE_DEV_PROJECT_REF wajib diisi untuk koneksi Supabase development")
	}
	if s.AppEnv == "test" {
		return errors.New("APP_ENV=test harus memakai database terisolasi; Go Core API saat ini memakai Supabase PostgreSQL")
	}
	if expectedRef != "" {
		identity := strings.ToLower(u.Hostname() + " " + u.User.Username())
		if !strings.Contains(identity, strings.ToLower(expectedRef)) {
			return errors.New("DATABASE_URL harus menunjuk project Supabase yang cocok dengan project ref")
		}
	}
	return nil
}

func validateMigrationDatabaseURL(s Settings, value string) error {
	if s.AppEnv != "development" {
		return errors.New("MIGRATION_DATABASE_URL hanya boleh dipakai dalam development")
	}
	if s.DatabaseURL == "" {
		return errors.New("DATABASE_URL wajib diisi agar app dan migrator menunjuk project development yang sama")
	}
	parse := func(raw string) (*url.URL, error) {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql" && u.Scheme != "postgresql+psycopg") || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || strings.Trim(u.Path, "/") == "" {
			return nil, errors.New("MIGRATION_DATABASE_URL harus berupa connection string PostgreSQL lengkap")
		}
		password, ok := u.User.Password()
		if !ok || password == "" {
			return nil, errors.New("MIGRATION_DATABASE_URL harus menyertakan password role migrator")
		}
		sslmode := strings.ToLower(u.Query().Get("sslmode"))
		if sslmode != "require" && sslmode != "verify-ca" && sslmode != "verify-full" {
			return nil, errors.New("MIGRATION_DATABASE_URL harus mewajibkan TLS lewat sslmode")
		}
		return u, nil
	}
	runtimeURL, err := url.Parse(strings.TrimSpace(s.DatabaseURL))
	if err != nil || runtimeURL.User == nil {
		return errors.New("DATABASE_URL harus berupa connection string PostgreSQL lengkap untuk migrasi development")
	}
	migratorURL, err := parse(value)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.SplitN(migratorURL.User.Username(), ".", 2)[0]) != "algentra_migrator" {
		return errors.New("MIGRATION_DATABASE_URL harus memakai role development algentra_migrator")
	}
	projectRef := strings.ToLower(strings.TrimSpace(s.SupabaseDevRef))
	if projectRef == "" {
		return errors.New("SUPABASE_DEV_PROJECT_REF wajib diisi untuk menjalankan migrasi development")
	}
	belongsToProject := func(u *url.URL) bool {
		host := strings.ToLower(u.Hostname())
		user := strings.ToLower(u.User.Username())
		return strings.Contains(host, projectRef) || strings.HasSuffix(user, "."+projectRef)
	}
	if runtimeURL.Path != migratorURL.Path || runtimeURL.User.Username() == migratorURL.User.Username() || !belongsToProject(runtimeURL) || !belongsToProject(migratorURL) {
		return errors.New("DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama dengan role berbeda")
	}
	return nil
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func normalizeOrigins(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	var origins []string
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("WEBSOCKET_ALLOWED_ORIGINS berisi origin HTTP(S) yang tidak valid")
		}
		canonical := strings.ToLower(u.Scheme + "://" + u.Host)
		if _, ok := seen[canonical]; !ok {
			seen[canonical] = struct{}{}
			origins = append(origins, canonical)
		}
	}
	return origins, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s harus true atau false", key)
	}
	return parsed, nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func hasPlaceholder(value string, markers ...string) bool {
	value = strings.ToLower(value)
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
