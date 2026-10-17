package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func registerDownloads(mux *http.ServeMux, configuredRoot string) {
	root := configuredRoot
	if root == "" {
		root, _ = os.Executable()
		root = filepath.Dir(root)
	}
	mux.HandleFunc("GET /downloads/MT5FollowerCopyEA.mq5", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ClientIDFromRequest(r); !ok {
			writeError(w, http.StatusUnauthorized, "Login client diperlukan")
			return
		}
		serveEAFile(w, r, filepath.Join(root, "mt5", "experts", "MT5FollowerCopyEA.mq5"), "MT5FollowerCopyEA.mq5", "text/plain; charset=utf-8")
	})
	mux.HandleFunc("GET /downloads/MT5FollowerCopyEA.ex5", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ClientIDFromRequest(r); !ok {
			writeError(w, 401, "Login client diperlukan")
			return
		}
		serveCompiledEA(w, r, root, "MT5FollowerCopyEA.ex5")
	})
	for _, name := range []string{"TelegramSignalEA.mq5", "TelegramSignalEA.ex5"} {
		filename := name
		mux.HandleFunc("GET /downloads/"+filename, func(w http.ResponseWriter, r *http.Request) {
			if !adminDownloadAuthorized(w, r) {
				return
			}
			if filename[len(filename)-4:] == ".ex5" {
				serveCompiledEA(w, r, root, filename)
			} else {
				serveEAFile(w, r, filepath.Join(root, "mt5", "experts", filename), filename, "text/plain; charset=utf-8")
			}
		})
	}
}

func adminDownloadAuthorized(w http.ResponseWriter, r *http.Request) bool {
	state := SessionFromRequest(r)
	if state != nil {
		if user, ok := state.Values["admin_username"].(string); ok && user != "" {
			return true
		}
	}
	writeError(w, 401, "Login admin diperlukan")
	return false
}

func serveCompiledEA(w http.ResponseWriter, r *http.Request, root, name string) {
	sourceName := name[:len(name)-4] + ".mq5"
	binaryPath := filepath.Join(root, "mt5", "experts", name)
	sourcePath := filepath.Join(root, "mt5", "experts", sourceName)
	binary, binaryErr := os.Stat(binaryPath)
	source, sourceErr := os.Stat(sourcePath)
	if binaryErr != nil || sourceErr != nil || source.ModTime().After(binary.ModTime()) {
		writeError(w, http.StatusGone, "Binary EA belum diperbarui. Unduh source .mq5 dan compile di MetaEditor.")
		return
	}
	serveEAFile(w, r, binaryPath, name, "application/octet-stream")
}

func serveEAFile(w http.ResponseWriter, r *http.Request, path, name, contentType string) {
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "File EA tidak ditemukan")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "File EA tidak ditemukan")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeContent(w, r, name, time.Time{}, file)
}
