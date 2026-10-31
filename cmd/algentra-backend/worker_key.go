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

type storedWorkerKey struct {
	APIKey string `json:"api_key"`
}

func loadOrCreateCopierWorkerAPIKey(configured, path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		var stored storedWorkerKey
		if err := json.Unmarshal(data, &stored); err != nil {
			return "", fmt.Errorf("baca key worker tersimpan: %w", err)
		}
		key := strings.TrimSpace(stored.APIKey)
		if len(key) < 32 {
			return "", errors.New("key worker tersimpan tidak valid")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("baca key worker tersimpan: %w", err)
	}

	key := strings.TrimSpace(configured)
	if key == "" {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("buat key worker: %w", err)
		}
		key = hex.EncodeToString(bytes)
	}
	if len(key) < 32 {
		return "", errors.New("COPIER_WORKER_API_KEY harus minimal 32 karakter")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("buat direktori key worker: %w", err)
	}
	data, err := json.Marshal(storedWorkerKey{APIKey: key})
	if err != nil {
		return "", fmt.Errorf("encode key worker: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".copier-worker-key-*.tmp")
	if err != nil {
		return "", fmt.Errorf("buat file sementara key worker: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return "", fmt.Errorf("batasi izin file key worker: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("tulis key worker: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", fmt.Errorf("simpan key worker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("tutup file key worker: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return "", fmt.Errorf("pasang file key worker: %w", err)
	}
	return key, nil
}
