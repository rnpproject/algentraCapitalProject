package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type geminiRuntime struct {
	mu       sync.RWMutex
	apiKey   string
	model    string
	filePath string
}

type storedGeminiSettings struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

func newGeminiRuntime(apiKey, model, filePath string) *geminiRuntime {
	runtime := &geminiRuntime{apiKey: strings.TrimSpace(apiKey), model: strings.TrimSpace(model), filePath: filePath}
	if runtime.model == "" {
		runtime.model = "gemini-3.5-flash-lite"
	}
	if filePath == "" {
		return runtime
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return runtime
	}
	var stored storedGeminiSettings
	if json.Unmarshal(data, &stored) != nil {
		return runtime
	}
	if strings.TrimSpace(stored.APIKey) != "" {
		runtime.apiKey = strings.TrimSpace(stored.APIKey)
	}
	if strings.TrimSpace(stored.Model) != "" && geminiModelPattern.MatchString(stored.Model) {
		runtime.model = stored.Model
	}
	return runtime
}

func (runtime *geminiRuntime) snapshot() storedGeminiSettings {
	if runtime == nil {
		return storedGeminiSettings{}
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return storedGeminiSettings{APIKey: runtime.apiKey, Model: runtime.model}
}

func (runtime *geminiRuntime) update(apiKey, model *string) error {
	if runtime == nil {
		return os.ErrInvalid
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	updated := storedGeminiSettings{APIKey: runtime.apiKey, Model: runtime.model}
	if apiKey != nil && strings.TrimSpace(*apiKey) != "" {
		updated.APIKey = strings.TrimSpace(*apiKey)
	}
	if model != nil && strings.TrimSpace(*model) != "" {
		updated.Model = strings.TrimSpace(*model)
	}
	if runtime.filePath != "" {
		data, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(runtime.filePath), 0700); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(runtime.filePath), ".gemini-settings-*.tmp")
		if err != nil {
			return err
		}
		temporaryName := temporary.Name()
		defer os.Remove(temporaryName)
		if err := temporary.Chmod(0600); err != nil {
			temporary.Close()
			return err
		}
		if _, err := temporary.Write(data); err != nil {
			temporary.Close()
			return err
		}
		if err := temporary.Sync(); err != nil {
			temporary.Close()
			return err
		}
		if err := temporary.Close(); err != nil {
			return err
		}
		if err := os.Rename(temporaryName, runtime.filePath); err != nil {
			return err
		}
	}
	runtime.apiKey, runtime.model = updated.APIKey, updated.Model
	return nil
}
