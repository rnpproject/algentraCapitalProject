package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rnpproject/algentra-backend/internal/auth"
	"github.com/rnpproject/algentra-backend/internal/config"
	"github.com/rnpproject/algentra-backend/internal/database"
	"github.com/rnpproject/algentra-backend/internal/httpapi"
	telegramclient "github.com/rnpproject/algentra-backend/internal/telegram"
)

func main() {
	projectRoot := os.Getenv("PROJECT_ROOT")
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	settings, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi service tidak valid", "error", err)
		os.Exit(1)
	}
	optionalIntegrationsPath := filepath.Join(projectRoot, "backend", "data", "integration-secrets", "optional-integrations.json")
	if err := loadOptionalIntegrationSettings(optionalIntegrationsPath, &settings); err != nil {
		slog.Error("konfigurasi integrasi opsional tidak valid", "error", err)
		os.Exit(1)
	}
	telegramSettingsPath := filepath.Join(projectRoot, "backend", "data", "integration-secrets", "telegram.json")
	if err := loadTelegramSettings(telegramSettingsPath, &settings); err != nil {
		slog.Error("konfigurasi Telegram tidak valid", "error", err)
		os.Exit(1)
	}
	appSecret, err := loadOrCreateAppSecret(settings.AppSecretKey, filepath.Join(projectRoot, "backend", "data", "integration-secrets", "app-secret.json"))
	if err != nil {
		slog.Error("application secret tidak dapat disiapkan", "error", err)
		os.Exit(1)
	}
	settings.AppSecretKey = appSecret
	if settings.DatabaseURL == "" {
		slog.Error("DATABASE_URL wajib diisi untuk Core API Go")
		os.Exit(1)
	}
	projectRef := settings.SupabaseProjectRef
	if settings.AppEnv == "development" {
		projectRef = settings.SupabaseDevRef
	}
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	pool, err := database.Open(startupCtx, settings.DatabaseURL)
	startupCancel()
	if err != nil {
		slog.Error("gagal menyiapkan koneksi database")
		os.Exit(1)
	}
	defer pool.Close()
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = database.CheckReady(readyCtx, pool, settings.AppEnv, projectRef)
	readyCancel()
	if err != nil {
		slog.Error("database belum siap atau schema tidak sesuai", "error", err)
		os.Exit(1)
	}
	var adminExists bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM admin_users WHERE username = $1)`, settings.AdminUsername).Scan(&adminExists); err != nil {
		slog.Error("akun admin tidak dapat diperiksa", "error", err)
		os.Exit(1)
	}
	adminPassword := settings.AdminPassword
	autoAdminPasswordPath := ""
	if !adminExists && adminPassword == "" {
		if settings.AppEnv != "development" {
			slog.Error("ADMIN_PASSWORD wajib diisi untuk bootstrap admin production")
			os.Exit(1)
		}
		autoAdminPasswordPath = filepath.Join(projectRoot, "backend", "data", "integration-secrets", "admin-bootstrap.json")
		adminPassword, err = loadOrCreateAdminBootstrapPassword(autoAdminPasswordPath)
		if err != nil {
			slog.Error("password bootstrap admin tidak dapat disiapkan", "error", err)
			os.Exit(1)
		}
	}
	adminCtx, adminCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = auth.EnsureAdmin(adminCtx, pool, settings.AdminUsername, adminPassword)
	adminCancel()
	if err != nil {
		slog.Error("akun admin tidak dapat disiapkan", "error", err)
		os.Exit(1)
	}
	if !adminExists && settings.AppEnv == "development" {
		fmt.Fprintf(os.Stderr, "Password awal admin (simpan sekarang): %s\n", adminPassword)
	}
	if autoAdminPasswordPath != "" {
		if err := os.Remove(autoAdminPasswordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("password bootstrap admin tersimpan tidak dapat dihapus setelah akun dibuat", "error", err)
		}
	}
	settingsCtx, settingsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = httpapi.EnsureStartupSettings(settingsCtx, pool)
	settingsCancel()
	if err != nil {
		slog.Error("pengaturan awal tidak dapat disiapkan", "error", err)
		os.Exit(1)
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = httpapi.PurgeExpiredConversations(cleanupCtx, pool)
	cleanupCancel()
	if err != nil {
		slog.Error("percakapan CS kedaluwarsa tidak dapat dibersihkan", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go httpapi.CleanupExpiredConversationsPeriodically(ctx, pool)
	eaAPIKey, err := loadOrCreateEAAPIKey(settings.EAAPIKey, filepath.Join(projectRoot, "backend", "data", "integration-secrets", "ea.json"))
	if err != nil {
		slog.Error("EA API key tidak dapat disiapkan", "error", err)
		os.Exit(1)
	}
	workerAPIKey, err := loadOrCreateCopierWorkerAPIKey(settings.CopierWorkerAPIKey, filepath.Join(projectRoot, "backend", "data", "integration-secrets", "copier-worker.json"))
	if err != nil {
		slog.Error("copier worker API key tidak dapat disiapkan", "error", err)
		os.Exit(1)
	}
	tgManager := telegramclient.NewManager(settings.TelegramAPIID, settings.TelegramAPIHash, filepath.Join(projectRoot, "backend", "data", "telegram_user.gotd.session"))
	geminiSettingsPath := filepath.Join(projectRoot, "backend", "data", "integration-secrets", "gemini.json")
	handler := httpapi.NewHandler(pool, httpapi.ServerConfig{
		Environment:             settings.AppEnv,
		ProjectRef:              projectRef,
		ServiceName:             "algentra-capital",
		CookieSecure:            settings.CookieSecure,
		WebSocketOrigins:        settings.WebSocketOrigins,
		TelegramManager:         tgManager,
		AppSecretKey:            settings.AppSecretKey,
		WorkerAPIKey:            workerAPIKey,
		EAAPIKey:                eaAPIKey,
		PublicMusicURL:          settings.PublicMusicURL,
		GeminiAPIKey:            settings.GeminiAPIKey,
		GeminiModel:             settings.GeminiModel,
		GeminiSettingsPath:      geminiSettingsPath,
		PaymentBankName:         settings.PaymentBankName,
		PaymentNumber:           settings.PaymentNumber,
		PaymentName:             settings.PaymentName,
		MidtransConfigured:      settings.MidtransConfigured,
		MidtransServerKey:       settings.MidtransServerKey,
		MidtransMerchantID:      settings.MidtransMerchantID,
		MidtransProduction:      settings.MidtransProduction,
		MidtransNotificationURL: settings.MidtransNotificationURL,
		TelegramBotToken:        settings.TelegramBotToken,
		TelegramAdminChatID:     settings.TelegramAdminChatID,
		BotContext:              ctx,
		EmailConfigured:         settings.EmailConfigured,
		EmailSMTPHost:           settings.EmailSMTPHost,
		EmailSMTPPort:           settings.EmailSMTPPort,
		EmailSMTPUsername:       settings.EmailSMTPUsername,
		EmailSMTPPassword:       settings.EmailSMTPPassword,
		EmailFrom:               settings.EmailFrom,
		EmailSMTPStartTLS:       settings.EmailSMTPStartTLS,
		ProjectRoot:             projectRoot,
		Contact: httpapi.ContactConfig{
			PersonName:    settings.ContactPersonName,
			WhatsApp:      settings.ContactWhatsApp,
			Email:         settings.ContactEmail,
			Instagram:     settings.ContactInstagram,
			Telegram:      settings.ContactTelegram,
			TelegramAdmin: settings.ContactTelegramAdmin,
			TikTok:        settings.ContactTikTok,
			Website:       settings.ContactWebsite,
			Facebook:      settings.ContactFacebook,
			LinkedIn:      settings.ContactLinkedIn,
			X:             settings.ContactX,
			YouTube:       settings.ContactYouTube,
		},
		Pricing: httpapi.ServicePricing{
			ManagementFeeIDR: settings.ManagementFeeIDR,
			VPSOptions: func() []httpapi.VPSPlan {
				plans := make([]httpapi.VPSPlan, 0, len(settings.VPSOptions))
				for _, plan := range settings.VPSOptions {
					plans = append(plans, httpapi.VPSPlan{Code: plan.Code, Name: plan.Name, VCPU: plan.VCPU, RAM: plan.RAM, Disk: plan.Disk, PriceIDR: plan.PriceIDR})
				}
				return plans
			}(),
		},
	})
	if err := tgManager.Start(ctx); err != nil {
		slog.Warn("listener Telegram Go tidak dapat dimulai", "error", err)
	}
	server := &http.Server{
		Addr:              settings.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverError := make(chan error, 1)
	go func() {
		slog.Info("Core API Go mulai", "address", settings.HTTPAddr, "environment", settings.AppEnv)
		serverError <- server.ListenAndServe()
	}()
	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Core API berhenti", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown Core API gagal", "error", err)
		}
	}
}
