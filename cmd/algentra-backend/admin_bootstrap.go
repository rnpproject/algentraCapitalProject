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

type storedAdminBootstrap struct {
	Password string `json:"password"`
}

func loadOrCreateAdminBootstrapPassword(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		var stored storedAdminBootstrap
		if err := json.Unmarshal(data, &stored); err != nil {
			return "", fmt.Errorf("baca password bootstrap admin tersimpan: %w", err)
		}
		password := strings.TrimSpace(stored.Password)
		if len(password) < 24 {
			return "", errors.New("password bootstrap admin tersimpan tidak valid")
		}
		return password, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("baca password bootstrap admin tersimpan: %w", err)
	}

	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("buat password bootstrap admin: %w", err)
	}
	password := hex.EncodeToString(bytes)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("buat direktori password bootstrap admin: %w", err)
	}
	data, err := json.Marshal(storedAdminBootstrap{Password: password})
	if err != nil {
		return "", fmt.Errorf("encode password bootstrap admin: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".admin-bootstrap-*.tmp")
	if err != nil {
		return "", fmt.Errorf("buat file sementara password bootstrap admin: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return "", fmt.Errorf("batasi izin file password bootstrap admin: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("tulis password bootstrap admin: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", fmt.Errorf("simpan password bootstrap admin: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("tutup file password bootstrap admin: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", fmt.Errorf("pasang file password bootstrap admin: %w", err)
	}
	return password, nil
}
