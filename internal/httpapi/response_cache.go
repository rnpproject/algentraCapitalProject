package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

type cachedResponse struct {
	header  http.Header
	status  int
	body    []byte
	expires time.Time
}

// responseCache shares short-lived, read-only API snapshots. Concurrent misses
// for one key wait for one handler; different keys remain independent.
type responseCacheCall struct {
	done chan struct{}
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]cachedResponse
	loading map[string]*responseCacheCall
}

const maxCachedResponses = 512

func newResponseCache() *responseCache {
	return &responseCache{entries: make(map[string]cachedResponse), loading: make(map[string]*responseCacheCall)}
}

func (cache *responseCache) serve(key string, ttl time.Duration, next http.Handler, w http.ResponseWriter, r *http.Request) {
	for {
		now := time.Now()
		cache.mu.Lock()
		if entry, ok := cache.entries[key]; ok && now.Before(entry.expires) {
			cache.mu.Unlock()
			copyResponse(w, entry.header, entry.status, entry.body)
			return
		}
		if call := cache.loading[key]; call != nil {
			cache.mu.Unlock()
			select {
			case <-call.done:
				continue
			case <-r.Context().Done():
				return
			}
		}
		call := &responseCacheCall{done: make(chan struct{})}
		cache.loading[key] = call
		cache.mu.Unlock()

		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)
		result := recorder.Result()
		_ = result.Body.Close()
		header := result.Header.Clone()
		body := bytes.Clone(recorder.Body.Bytes())
		finishedAt := time.Now()

		cache.mu.Lock()
		if result.StatusCode == http.StatusOK {
			if _, exists := cache.entries[key]; !exists && len(cache.entries) >= maxCachedResponses {
				oldestKey := ""
				var oldestExpiry time.Time
				for existingKey, existing := range cache.entries {
					if !finishedAt.Before(existing.expires) {
						delete(cache.entries, existingKey)
						continue
					}
					if oldestKey == "" || existing.expires.Before(oldestExpiry) {
						oldestKey, oldestExpiry = existingKey, existing.expires
					}
				}
				if len(cache.entries) >= maxCachedResponses && oldestKey != "" {
					delete(cache.entries, oldestKey)
				}
			}
			cache.entries[key] = cachedResponse{
				header:  header,
				status:  result.StatusCode,
				body:    body,
				expires: finishedAt.Add(ttl),
			}
		} else {
			delete(cache.entries, key)
		}
		delete(cache.loading, key)
		close(call.done)
		cache.mu.Unlock()
		copyResponse(w, header, result.StatusCode, body)
		return
	}
}

func copyResponse(w http.ResponseWriter, header http.Header, status int, body []byte) {
	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
