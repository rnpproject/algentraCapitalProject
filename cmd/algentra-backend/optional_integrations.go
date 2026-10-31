package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"

	"github.com/rnpproject/algentra-backend/internal/config"
)

type storedOptionalIntegrations struct {
	PaymentBankName         string `json:"payment_bank_name"`
	PaymentNumber           string `json:"payment_number"`
	PaymentName             string `json:"payment_name"`
	MidtransServerKey       string `json:"midtrans_server_key"`
	MidtransMerchantID      string `json:"midtrans_merchant_id"`
	MidtransProduction      bool   `json:"midtrans_production"`
	MidtransNotificationURL string `json:"midtrans_notification_url"`
	EmailSMTPHost           string `json:"email_smtp_host"`
	EmailSMTPPort           int    `json:"email_smtp_port"`
	EmailSMTPUsername       string `json:"email_smtp_username"`
	EmailSMTPPassword       string `json:"email_smtp_password"`
	EmailFrom               string `json:"email_from"`
	EmailSMTPStartTLS       *bool  `json:"email_smtp_starttls"`
}

func loadOptionalIntegrationSettings(path string, settings *config.Settings) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("baca konfigurasi integrasi opsional: %w", err)
	}
	var stored storedOptionalIntegrations
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("decode konfigurasi integrasi opsional: %w", err)
	}
	stored.MidtransServerKey = strings.TrimSpace(stored.MidtransServerKey)
	stored.MidtransMerchantID = strings.TrimSpace(stored.MidtransMerchantID)
	stored.MidtransNotificationURL = strings.TrimSpace(stored.MidtransNotificationURL)
	if (stored.MidtransServerKey == "") != (stored.MidtransMerchantID == "") {
		return errors.New("Midtrans Server Key dan Merchant ID harus dikonfigurasi bersama")
	}
	if stored.MidtransNotificationURL != "" {
		notificationURL, err := url.ParseRequestURI(stored.MidtransNotificationURL)
		if err != nil || !strings.EqualFold(notificationURL.Scheme, "https") || notificationURL.Hostname() == "" || notificationURL.User != nil {
			return errors.New("URL notifikasi Midtrans harus berupa URL HTTPS yang valid")
		}
	}

	stored.EmailSMTPHost = strings.TrimSpace(stored.EmailSMTPHost)
	stored.EmailSMTPUsername = strings.TrimSpace(stored.EmailSMTPUsername)
	stored.EmailFrom = strings.TrimSpace(stored.EmailFrom)
	configuredEmail := 0
	for _, value := range []string{stored.EmailSMTPHost, stored.EmailSMTPUsername, stored.EmailSMTPPassword, stored.EmailFrom} {
		if value != "" {
			configuredEmail++
		}
	}
	if configuredEmail != 0 && configuredEmail != 4 {
		return errors.New("host, username, password, dan alamat pengirim SMTP harus diisi bersama")
	}
	if configuredEmail == 4 {
		if stored.EmailSMTPPort == 0 {
			stored.EmailSMTPPort = 587
		}
		if stored.EmailSMTPPort < 1 || stored.EmailSMTPPort > 65535 {
			return errors.New("port SMTP harus antara 1 dan 65535")
		}
		if _, err := mail.ParseAddress(stored.EmailFrom); err != nil {
			return errors.New("alamat pengirim SMTP tidak valid")
		}
		settings.EmailConfigured = true
		settings.EmailSMTPHost = stored.EmailSMTPHost
		settings.EmailSMTPPort = stored.EmailSMTPPort
		settings.EmailSMTPUsername = stored.EmailSMTPUsername
		settings.EmailSMTPPassword = stored.EmailSMTPPassword
		settings.EmailFrom = stored.EmailFrom
		settings.EmailSMTPStartTLS = stored.EmailSMTPStartTLS == nil || *stored.EmailSMTPStartTLS
	}

	settings.PaymentBankName = strings.TrimSpace(stored.PaymentBankName)
	settings.PaymentNumber = strings.TrimSpace(stored.PaymentNumber)
	settings.PaymentName = strings.TrimSpace(stored.PaymentName)
	settings.MidtransServerKey = stored.MidtransServerKey
	settings.MidtransMerchantID = stored.MidtransMerchantID
	settings.MidtransProduction = stored.MidtransProduction
	settings.MidtransNotificationURL = stored.MidtransNotificationURL
	settings.MidtransConfigured = stored.MidtransServerKey != ""
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("validasi konfigurasi integrasi opsional: %w", err)
	}
	return nil
}
