package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type storedAppSecret struct {
	SecretKey string `json:"secret_key"`
}

func loadOrCreateAppSecret(configured, path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		var stored storedAppSecret
		if err := json.Unmarshal(data, &stored); err != nil {
			return "", fmt.Errorf("baca application secret tersimpan: %w", err)
		}
		secret := strings.TrimSpace(stored.SecretKey)
		if len(secret) < 32 {
			return "", errors.New("application secret tersimpan tidak valid")
		}
		return secret, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("baca application secret tersimpan: %w", err)
	}

	secret := strings.TrimSpace(configured)
	if secret == "" {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("buat application secret: %w", err)
		}
		secret = hex.EncodeToString(bytes)
	}
	if len(secret) < 32 {
		return "", errors.New("APP_SECRET_KEY harus minimal 32 karakter")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("buat direktori application secret: %w", err)
	}
	data, err := json.Marshal(storedAppSecret{SecretKey: secret})
	if err != nil {
		return "", fmt.Errorf("encode application secret: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".app-secret-*.tmp")
	if err != nil {
		return "", fmt.Errorf("buat file sementara application secret: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return "", fmt.Errorf("batasi izin file application secret: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("tulis application secret: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", fmt.Errorf("simpan application secret: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("tutup file application secret: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", fmt.Errorf("pasang file application secret: %w", err)
	}
	return secret, nil
}
