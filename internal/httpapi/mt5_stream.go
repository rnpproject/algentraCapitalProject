package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type mt5UpdateSubscriber struct {
	ownerID int64
	updates chan int64
}

type mt5UpdateHub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]mt5UpdateSubscriber
}

func newMT5UpdateHub() *mt5UpdateHub {
	return &mt5UpdateHub{subscribers: make(map[uint64]mt5UpdateSubscriber)}
}

func (h *mt5UpdateHub) subscribe(ownerID int64) (<-chan int64, func()) {
	updates := make(chan int64, 1)
	if h == nil {
		close(updates)
		return updates, func() {}
	}
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.subscribers[id] = mt5UpdateSubscriber{ownerID: ownerID, updates: updates}
	h.mu.Unlock()
	return updates, func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		h.mu.Unlock()
	}
}

func (h *mt5UpdateHub) publish(ownerID, accountID int64) {
	if h == nil || ownerID <= 0 || accountID <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, subscriber := range h.subscribers {
		if subscriber.ownerID != ownerID {
			continue
		}
		select {
		case subscriber.updates <- accountID:
		default:
			// A refresh event represents current database state, so coalesce a
			// backed-up queue to its latest account update.
			select {
			case <-subscriber.updates:
			default:
			}
			select {
			case subscriber.updates <- accountID:
			default:
			}
		}
	}
}

func (api clientMT5API) accountUpdatesStream(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	ownerID, ok := requireClient(ctx, api.pool, w, r)
	cancel()
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming tidak didukung")
		return
	}
	updates, unsubscribe := api.updates.subscribe(ownerID)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "event: mt5-update\ndata: {\"account_id\":0,\"initial\":true}\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case accountID, open := <-updates:
			if !open {
				return
			}
			payload, err := json.Marshal(map[string]any{"account_id": accountID})
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: mt5-update\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
