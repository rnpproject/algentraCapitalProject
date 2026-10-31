package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/rnpproject/algentra-backend/internal/config"
)

type storedTelegramSettings struct {
	APIID       int    `json:"api_id"`
	APIHash     string `json:"api_hash"`
	BotToken    string `json:"bot_token"`
	AdminChatID string `json:"admin_chat_id"`
}

func loadTelegramSettings(path string, settings *config.Settings) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var stored storedTelegramSettings
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	stored.APIHash = strings.TrimSpace(stored.APIHash)
	stored.BotToken = strings.TrimSpace(stored.BotToken)
	stored.AdminChatID = strings.TrimSpace(stored.AdminChatID)
	if (stored.APIID == 0) != (stored.APIHash == "") || stored.APIID < 0 {
		return errors.New("Telegram API ID dan API hash harus diisi berpasangan")
	}
	if stored.APIHash != "" {
		if len(stored.APIHash) != 32 {
			return errors.New("Telegram API hash harus 32 karakter hexadecimal")
		}
		if _, err := hex.DecodeString(stored.APIHash); err != nil {
			return errors.New("Telegram API hash harus 32 karakter hexadecimal")
		}
	}
	if (stored.BotToken == "") != (stored.AdminChatID == "") {
		return errors.New("Telegram bot token dan admin chat ID harus diisi berpasangan")
	}
	if stored.AdminChatID != "" {
		if id, err := strconv.ParseInt(stored.AdminChatID, 10, 64); err != nil || id < 1 {
			return errors.New("Telegram admin chat ID harus positif")
		}
	}
	if stored.APIID > 0 {
		settings.TelegramAPIID = stored.APIID
		settings.TelegramAPIHash = stored.APIHash
	}
	if stored.BotToken != "" {
		settings.TelegramBotToken = stored.BotToken
		settings.TelegramAdminChatID = stored.AdminChatID
	}
	return nil
}
