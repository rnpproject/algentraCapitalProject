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

type storedEAKey struct {
	APIKey string `json:"api_key"`
}

func loadOrCreateEAAPIKey(configured, path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		var stored storedEAKey
		if err := json.Unmarshal(data, &stored); err != nil {
			return "", fmt.Errorf("baca key EA tersimpan: %w", err)
		}
		key := strings.TrimSpace(stored.APIKey)
		if len(key) < 24 {
			return "", errors.New("key EA tersimpan tidak valid")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("baca key EA tersimpan: %w", err)
	}

	key := strings.TrimSpace(configured)
	if key == "" {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("buat key EA: %w", err)
		}
		key = hex.EncodeToString(bytes)
	}
	if len(key) < 24 {
		return "", errors.New("EA_API_KEY harus minimal 24 karakter")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("buat direktori key EA: %w", err)
	}
	data, err := json.Marshal(storedEAKey{APIKey: key})
	if err != nil {
		return "", fmt.Errorf("encode key EA: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ea-key-*.tmp")
	if err != nil {
		return "", fmt.Errorf("buat file sementara key EA: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return "", fmt.Errorf("batasi izin file key EA: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("tulis key EA: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", fmt.Errorf("simpan key EA: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("tutup file key EA: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", fmt.Errorf("pasang file key EA: %w", err)
	}
	return key, nil
}
