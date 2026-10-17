package telegram

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
	_ "modernc.org/sqlite"
)

var ErrNotConfigured = errors.New("TELEGRAM_API_ID dan TELEGRAM_API_HASH belum dikonfigurasi")
var ErrNotConnected = errors.New("Telegram belum terhubung")
var ErrInvalidPhone = errors.New("Nomor telepon harus dalam format internasional, contoh +628123456789")
var ErrCodeNotRequested = errors.New("Minta kode Telegram terlebih dahulu")
var Err2FAState = errors.New("Login belum meminta password 2FA")

type Manager struct {
	apiID      int
	apiHash    string
	session    string
	client     *telegram.Client
	authClient *auth.Client
	api        *tg.Client

	mu             sync.RWMutex
	started        bool
	connected      bool
	runCtx         context.Context
	pendingPhone   string
	phoneCodeHash  string
	awaiting2FA    bool
	accountName    string
	lastRunFailure string
	messageHandler func(context.Context, IncomingMessage) error
	messageQueue   chan IncomingMessage
}

type IncomingMessage struct {
	GroupID    string
	MessageID  string
	SenderID   string
	SenderName string
	Text       string
	ReplyText  string
	replyID    int
	replyPeer  tg.InputPeerClass
	CreatedAt  time.Time
}

func NewManager(apiID int, apiHash, sessionPath string) *Manager {
	return &Manager{apiID: apiID, apiHash: strings.TrimSpace(apiHash), session: sessionPath, messageQueue: make(chan IncomingMessage, 128)}
}

func (m *Manager) Configured() bool { return m.apiID > 0 && len(m.apiHash) == 32 }

func (m *Manager) Start(ctx context.Context) error {
	if !m.Configured() {
		return nil
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if m.session == "" {
		return errors.New("path session Telegram kosong")
	}
	if err := os.MkdirAll(filepath.Dir(m.session), 0o700); err != nil {
		return fmt.Errorf("buat direktori session Telegram: %w", err)
	}
	if err := os.Chmod(filepath.Dir(m.session), 0o700); err != nil && runtimeIsUnix() {
		return fmt.Errorf("batasi akses direktori session Telegram: %w", err)
	}
	if migrated, err := m.migrateTelethonSession(ctx); err != nil {
		slog.Warn("Telegram session lama tidak dapat dikonversi", "error_type", fmt.Sprintf("%T", err))
	} else if migrated {
		slog.Info("Telegram session Telethon lama dikonversi ke format Go")
	}
	m.mu.Lock()
	m.started = true
	m.runCtx = ctx
	m.mu.Unlock()
	go m.consumeMessages(ctx)
	go m.run(ctx)
	return nil
}

func (m *Manager) migrateTelethonSession(ctx context.Context) (bool, error) {
	if _, err := os.Stat(m.session); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	legacyPath := m.telethonLegacyPath()
	if legacyPath == "" {
		return false, nil
	}
	if _, err := os.Stat(m.telethonMarkerPath()); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	path := filepath.ToSlash(legacyPath)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var dcID, port int
	var address string
	var authKey []byte
	if err := db.QueryRowContext(ctx, `SELECT dc_id, server_address, port, auth_key FROM sessions LIMIT 1`).Scan(&dcID, &address, &port, &authKey); err != nil {
		return false, err
	}
	if dcID <= 0 || port <= 0 || port > 65535 || len(authKey) != len(crypto.Key{}) || net.ParseIP(address) == nil {
		return false, errors.New("data sesi Telethon tidak valid")
	}
	var key crypto.Key
	copy(key[:], authKey)
	id := key.ID()
	data := &session.Data{DC: dcID, Addr: net.JoinHostPort(address, fmt.Sprint(port)), AuthKey: append([]byte(nil), authKey...), AuthKeyID: id[:]}
	loader := session.Loader{Storage: &session.FileStorage{Path: m.session}}
	if err := loader.Save(ctx, data); err != nil {
		return false, err
	}
	return true, m.markTelethonSessionConsumed()
}

func (m *Manager) telethonLegacyPath() string {
	legacyPath := strings.TrimSuffix(m.session, ".gotd.session") + ".session"
	if legacyPath == m.session {
		return ""
	}
	return legacyPath
}

func (m *Manager) telethonMarkerPath() string { return m.session + ".telethon-migrated" }

func (m *Manager) markTelethonSessionConsumed() error {
	legacyPath := m.telethonLegacyPath()
	if legacyPath == "" {
		return nil
	}
	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return os.WriteFile(m.telethonMarkerPath(), []byte("consumed\n"), 0o600)
}

func (m *Manager) run(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		dispatcher := tg.NewUpdateDispatcher()
		dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
			m.handleMessageUpdate(ctx, entities, update.Message)
			return nil
		})
		dispatcher.OnNewChannelMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewChannelMessage) error {
			m.handleMessageUpdate(ctx, entities, update.Message)
			return nil
		})
		client := telegram.NewClient(m.apiID, m.apiHash, telegram.Options{
			SessionStorage: &session.FileStorage{Path: m.session},
			UpdateHandler:  dispatcher,
			OnConnectionState: func(state telegram.ConnectionState) {
				m.mu.Lock()
				m.connected = state == telegram.ConnectionStateReady
				m.mu.Unlock()
			},
			OnDead: func(error) {
				m.mu.Lock()
				m.connected = false
				m.mu.Unlock()
			},
		})
		api := client.API()
		m.mu.Lock()
		m.client, m.api = client, api
		m.authClient = auth.NewClient(api, rand.Reader, m.apiID, m.apiHash)
		m.mu.Unlock()
		err := client.Run(ctx, func(runCtx context.Context) error {
			m.mu.Lock()
			m.connected = true
			m.lastRunFailure = ""
			m.mu.Unlock()
			if status, err := auth.NewClient(api, rand.Reader, m.apiID, m.apiHash).Status(runCtx); err == nil && status.Authorized {
				m.mu.Lock()
				m.accountName = telegramUserName(status.User)
				m.mu.Unlock()
			}
			<-runCtx.Done()
			return nil
		})
		m.mu.Lock()
		m.connected = false
		if err != nil && ctx.Err() == nil {
			m.lastRunFailure = fmt.Sprintf("%T", err)
		}
		m.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func (m *Manager) SetMessageHandler(handler func(context.Context, IncomingMessage) error) {
	m.mu.Lock()
	m.messageHandler = handler
	m.mu.Unlock()
}

func (m *Manager) handleMessageUpdate(ctx context.Context, entities tg.Entities, messageClass tg.MessageClass) {
	message, ok := messageClass.(*tg.Message)
	if !ok || message.Out || strings.TrimSpace(message.Message) == "" {
		return
	}
	var groupID string
	var replyPeer tg.InputPeerClass
	switch peer := message.PeerID.(type) {
	case *tg.PeerChat:
		groupID = fmt.Sprint(-peer.ChatID)
		replyPeer = &tg.InputPeerChat{ChatID: peer.ChatID}
	case *tg.PeerChannel:
		groupID = fmt.Sprint(-1000000000000 - peer.ChannelID)
		if channel, found := entities.Channels[peer.ChannelID]; found && channel != nil {
			replyPeer = &tg.InputPeerChannel{ChannelID: peer.ChannelID, AccessHash: channel.AccessHash}
		}
	default:
		return
	}
	var senderID, senderName string
	if from, exists := message.GetFromID(); exists {
		if userPeer, isUser := from.(*tg.PeerUser); isUser {
			senderID = fmt.Sprint(userPeer.UserID)
			if sender, found := entities.Users[userPeer.UserID]; found && sender != nil {
				senderName = telegramUserName(sender)
			}
		}
	}
	created := time.Unix(int64(message.Date), 0).UTC()
	if message.Date <= 0 {
		created = time.Now().UTC()
	}
	event := IncomingMessage{GroupID: groupID, MessageID: fmt.Sprint(message.ID), SenderID: senderID, SenderName: senderName, Text: strings.TrimSpace(message.Message), CreatedAt: created, replyPeer: replyPeer}
	if reply, ok := message.GetReplyTo(); ok {
		if header, ok := reply.(*tg.MessageReplyHeader); ok {
			event.replyID, _ = header.GetReplyToMsgID()
		}
	}
	select {
	case m.messageQueue <- event:
	case <-ctx.Done():
	}
}

func (m *Manager) consumeMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-m.messageQueue:
			if message.replyID > 0 && message.replyPeer != nil {
				m.mu.RLock()
				api := m.api
				m.mu.RUnlock()
				if api != nil {
					if replies, err := query.NewQuery(api).Messages().GetMessages(ctx, message.replyPeer, message.replyID); err == nil && len(replies) > 0 {
						if reply, ok := replies[0].(*tg.Message); ok {
							message.ReplyText = strings.TrimSpace(reply.Message)
						}
					}
				}
			}
			m.mu.RLock()
			handler := m.messageHandler
			m.mu.RUnlock()
			if handler == nil {
				continue
			}
			if err := handler(ctx, message); err != nil {
				slog.Warn("telegram message processor failed", "error_type", fmt.Sprintf("%T", err))
			}
		}
	}
}

func (m *Manager) SendCode(ctx context.Context, phone string) (map[string]any, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 || len(phone) > 24 || !strings.HasPrefix(phone, "+") {
		return nil, ErrInvalidPhone
	}
	if err := m.waitConnected(ctx, 30*time.Second); err != nil {
		return nil, err
	}
	client := m.authService()
	if client == nil {
		return nil, ErrNotConnected
	}
	sent, err := client.SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return nil, err
	}
	code, ok := sent.(*tg.AuthSentCode)
	if !ok || code.PhoneCodeHash == "" {
		return nil, errors.New("Telegram tidak mengembalikan hash kode login")
	}
	m.mu.Lock()
	m.pendingPhone, m.phoneCodeHash, m.awaiting2FA = phone, code.PhoneCodeHash, false
	m.mu.Unlock()
	masked := phone[:3] + "••••" + phone[len(phone)-3:]
	return map[string]any{"status": "code_sent", "phone": masked, "requires_2fa": false}, nil
}

func (m *Manager) waitConnected(ctx context.Context, timeout time.Duration) error {
	if !m.Configured() {
		return ErrNotConfigured
	}
	m.mu.RLock()
	started := m.started
	m.mu.RUnlock()
	if !started {
		return ErrNotConnected
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.isConnected() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("waktu tunggu koneksi Telegram habis")
		case <-ticker.C:
		}
	}
}

func (m *Manager) VerifyCode(ctx context.Context, code string) (map[string]any, error) {
	m.mu.RLock()
	phone, hash := m.pendingPhone, m.phoneCodeHash
	m.mu.RUnlock()
	if phone == "" || hash == "" {
		return nil, ErrCodeNotRequested
	}
	if err := m.waitConnected(ctx, 30*time.Second); err != nil {
		return nil, err
	}
	client := m.authService()
	if client == nil {
		return nil, ErrNotConnected
	}
	result, err := client.SignIn(ctx, phone, strings.TrimSpace(code), hash)
	if errors.Is(err, auth.ErrPasswordAuthNeeded) {
		m.mu.Lock()
		m.awaiting2FA = true
		m.mu.Unlock()
		return map[string]any{"status": "two_factor_required", "requires_2fa": true}, nil
	}
	if err != nil {
		return nil, err
	}
	user, _ := result.User.(*tg.User)
	return m.finishLogin(user), nil
}

func (m *Manager) Verify2FA(ctx context.Context, password string) (map[string]any, error) {
	m.mu.RLock()
	awaiting := m.awaiting2FA
	m.mu.RUnlock()
	if !awaiting {
		return nil, Err2FAState
	}
	if err := m.waitConnected(ctx, 30*time.Second); err != nil {
		return nil, err
	}
	client := m.authService()
	if client == nil {
		return nil, ErrNotConnected
	}
	result, err := client.Password(ctx, password)
	if err != nil {
		return nil, err
	}
	user, _ := result.User.(*tg.User)
	return m.finishLogin(user), nil
}

func (m *Manager) finishLogin(user *tg.User) map[string]any {
	name := telegramUserName(user)
	m.mu.Lock()
	m.pendingPhone, m.phoneCodeHash, m.awaiting2FA, m.accountName = "", "", false, name
	m.mu.Unlock()
	return map[string]any{"status": "connected", "requires_2fa": false, "account_name": name}
}

func (m *Manager) Status(ctx context.Context) map[string]any {
	result := map[string]any{"configured": m.Configured(), "connected": false, "state": "disconnected", "account_name": nil}
	if !m.Configured() {
		return result
	}
	m.mu.RLock()
	connected, pending, awaiting, name := m.connected, m.pendingPhone != "", m.awaiting2FA, m.accountName
	m.mu.RUnlock()
	result["connected"] = connected && name != ""
	switch {
	case connected && name != "":
		result["state"], result["account_name"] = "connected", name
	case awaiting:
		result["state"] = "awaiting_2fa"
	case pending:
		result["state"] = "awaiting_code"
	}
	if connected {
		client := m.authService()
		if client == nil {
			return result
		}
		status, err := client.Status(ctx)
		if err != nil || !status.Authorized {
			result["connected"], result["account_name"] = false, nil
			if result["state"] == "connected" {
				result["state"] = "disconnected"
			}
		}
	}
	return result
}

func (m *Manager) Logout(ctx context.Context) (map[string]string, error) {
	if err := m.markTelethonSessionConsumed(); err != nil {
		return nil, fmt.Errorf("session Telegram lama tidak dapat ditandai sebelum logout: %w", err)
	}
	if api := m.rawAPI(); api != nil {
		if _, err := api.AuthLogOut(ctx); err != nil {
			return nil, err
		}
	}
	m.mu.Lock()
	m.pendingPhone, m.phoneCodeHash, m.accountName = "", "", ""
	m.awaiting2FA = false
	m.mu.Unlock()
	if err := os.Remove(m.session); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("hapus session Telegram: %w", err)
	}
	return map[string]string{"status": "logged_out"}, nil
}

func (m *Manager) Reconnect(ctx context.Context) (map[string]any, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	if !m.isConnected() {
		if err := m.waitConnected(ctx, 30*time.Second); err != nil {
			return nil, err
		}
	}
	status := m.Status(ctx)
	if connected, _ := status["connected"].(bool); connected {
		return map[string]any{"status": "connected", "connected": true, "account_name": status["account_name"]}, nil
	}
	return map[string]any{"status": "disconnected", "connected": false, "account_name": nil}, nil
}

func (m *Manager) Groups(ctx context.Context) ([]map[string]any, error) {
	if err := m.waitConnected(ctx, 30*time.Second); err != nil {
		return nil, err
	}
	api := m.rawAPI()
	if api == nil {
		return nil, ErrNotConnected
	}
	authClient := m.authService()
	if authClient == nil {
		return nil, ErrNotConnected
	}
	status, err := authClient.Status(ctx)
	if err != nil || !status.Authorized {
		return nil, ErrNotConnected
	}
	elements, err := query.NewQuery(api).GetDialogs().BatchSize(100).Collect(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]map[string]any, 0, len(elements))
	for _, element := range elements {
		dialog, ok := element.Dialog.(*tg.Dialog)
		if !ok || dialog.Peer == nil {
			continue
		}
		var id int64
		var name, username string
		switch peer := dialog.Peer.(type) {
		case *tg.PeerChat:
			id = -peer.ChatID
			if entity, found := element.Entities.Chat(peer.ChatID); found {
				name = entity.Title
			}
		case *tg.PeerChannel:
			id = -1000000000000 - peer.ChannelID
			if entity, found := element.Entities.Channel(peer.ChannelID); found {
				name, username = entity.Title, entity.Username
			}
		default:
			continue
		}
		if name == "" {
			name = "Telegram group"
		}
		groups = append(groups, map[string]any{"chat_id": fmt.Sprint(id), "name": name, "username": nullableTelegramString(username)})
	}
	return groups, nil
}

func (m *Manager) isConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *Manager) authService() *auth.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil
	}
	return m.authClient
}

func (m *Manager) rawAPI() *tg.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.connected {
		return nil
	}
	return m.api
}

func telegramUserName(user *tg.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name == "" {
		name = user.Username
	}
	return name
}

func nullableTelegramString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func runtimeIsUnix() bool { return os.PathSeparator == '/' }
