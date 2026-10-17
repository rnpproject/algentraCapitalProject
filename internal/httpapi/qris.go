package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type midtransReply struct {
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	MerchantID        string `json:"merchant_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	Actions           []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"actions"`
}

func (api paymentAPI) midtransNotification(w http.ResponseWriter, r *http.Request) {
	var payload midtransReply
	if err := decodeJSON(w, r, &payload, 64<<10); err != nil {
		writeError(w, 400, "Payload tidak valid")
		return
	}
	if api.midtransServerKey == "" || payload.OrderID == "" || payload.StatusCode == "" || payload.GrossAmount == "" || payload.SignatureKey == "" {
		writeError(w, 403, "Signature tidak valid")
		return
	}
	sum := sha512.Sum512([]byte(payload.OrderID + payload.StatusCode + payload.GrossAmount + api.midtransServerKey))
	expected := hex.EncodeToString(sum[:])
	if len(expected) != len(payload.SignatureKey) || subtle.ConstantTimeCompare([]byte(expected), []byte(payload.SignatureKey)) != 1 {
		writeError(w, 403, "Signature tidak valid")
		return
	}
	if api.midtransMerchantID != "" && strings.TrimSpace(payload.MerchantID) != api.midtransMerchantID {
		writeJSON(w, 200, map[string]any{"status": "MERCHANT_MISMATCH"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var orderID int64
	err := api.pool.QueryRow(ctx, `SELECT id FROM payment_orders WHERE midtrans_order_id=$1`, payload.OrderID).Scan(&orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 200, map[string]any{"status": "UNKNOWN_ORDER"})
		return
	}
	if err != nil {
		writeError(w, 503, "Status pembayaran tidak tersedia")
		return
	}
	_, status, _, err := api.applyMidtransStatus(ctx, orderID, payload)
	if err != nil {
		writeError(w, 503, "Status pembayaran tidak dapat disimpan")
		return
	}
	writeJSON(w, 200, map[string]any{"status": status})
}

func (api paymentAPI) midtransBase() string {
	if api.midtransProduction {
		return "https://api.midtrans.com"
	}
	return "https://api.sandbox.midtrans.com"
}
func (api paymentAPI) midtransRequest(ctx context.Context, method, path string, body any) (midtransReply, int, error) {
	var result midtransReply
	if api.midtransServerKey == "" {
		return result, 0, errors.New("midtrans is not configured")
	}
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return result, 0, err
		}
		input = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, api.midtransBase()+path, input)
	if err != nil {
		return result, 0, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(api.midtransServerKey + ":"))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if api.midtransNotificationURL != "" {
		req.Header.Set("X-Override-Notification", api.midtransNotificationURL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return result, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
			return result, resp.StatusCode, err
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	}
	return result, resp.StatusCode, nil
}

func (api paymentAPI) createQRIS(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := api.client(w, r)
	if !ok {
		return
	}
	id, ok := parsePaymentOrderID(r)
	if !ok {
		writeError(w, 404, "Pesanan tidak ditemukan")
		return
	}
	if api.midtransServerKey == "" {
		writeError(w, 503, "Pembayaran QRIS belum tersedia.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	tx, err := api.pool.Begin(ctx)
	if err != nil {
		writeError(w, 503, "Pesanan QRIS tidak tersedia")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("midtrans-charge:%d", id)); err != nil {
		writeError(w, 503, "Pesanan QRIS tidak tersedia")
		return
	}
	var owner int64
	var status, plan, email string
	var amount int64
	var method string
	var orderID, transactionID sql.NullString
	err = tx.QueryRow(ctx, `SELECT owner_id,status,plan,email,total_idr,payment_method,midtrans_order_id,midtrans_transaction_id FROM payment_orders WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &status, &plan, &email, &amount, &method, &orderID, &transactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "Pesanan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, 503, "Pesanan QRIS tidak tersedia")
		return
	}
	if owner != ownerID {
		writeError(w, 404, "Pesanan tidak ditemukan")
		return
	}
	if status != "PENDING_PAYMENT" {
		writeError(w, 409, "Pesanan ini tidak sedang menunggu pembayaran.")
		return
	}
	if method == "QRIS" && orderID.Valid {
		returnQRIS(w, 200, id, orderID.String, transactionID, amount, api.midtransBase())
		return
	}
	random := make([]byte, 10)
	if _, err := rand.Read(random); err != nil {
		writeError(w, 500, "QRIS tidak dapat dibuat")
		return
	}
	midtransID := fmt.Sprintf("ALG-%d-%s", id, hex.EncodeToString(random))
	payload := map[string]any{"payment_type": "qris", "transaction_details": map[string]any{"order_id": midtransID, "gross_amount": amount}, "item_details": []any{map[string]any{"id": plan, "price": amount, "quantity": 1, "name": "rnp-project"}}, "customer_details": map[string]any{"email": email}, "custom_field1": fmt.Sprint(id)}
	response, code, err := api.midtransRequest(ctx, http.MethodPost, "/v2/charge", payload)
	if err != nil {
		writeError(w, 502, "Tidak dapat menghubungi Midtrans. Coba lagi sebentar.")
		return
	}
	if code != 200 && code != 201 {
		writeError(w, 502, "Midtrans menolak pembuatan QRIS. Coba lagi sebentar.")
		return
	}
	qrURL := ""
	for _, action := range response.Actions {
		if action.Name == "generate-qr-code" {
			qrURL = action.URL
			break
		}
	}
	if qrURL == "" {
		writeError(w, 502, "Midtrans tidak mengembalikan kode QR.")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE payment_orders SET payment_method='QRIS',midtrans_order_id=$2,midtrans_transaction_id=NULLIF($3,''),midtrans_status=COALESCE(NULLIF($4,''),'pending'),updated_at=$5 WHERE id=$1`, id, midtransID, response.TransactionID, response.TransactionStatus, time.Now().UTC()); err != nil {
		writeError(w, 503, "Status QRIS tidak dapat disimpan")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		writeError(w, 503, "Status QRIS tidak dapat disimpan")
		return
	}
	writeJSON(w, 200, map[string]any{"order_id": id, "midtrans_order_id": midtransID, "qr_code_url": qrURL, "gross_amount": amount})
}

func returnQRIS(w http.ResponseWriter, status int, id int64, midtransID string, transactionID sql.NullString, amount int64, base string) {
	var qr any
	if transactionID.Valid && transactionID.String != "" {
		qr = base + "/v2/qris/" + url.PathEscape(transactionID.String) + "/qr-code"
	}
	writeJSON(w, status, map[string]any{"order_id": id, "midtrans_order_id": midtransID, "qr_code_url": qr, "gross_amount": amount})
}

func (api paymentAPI) qrisState(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := api.client(w, r)
	if !ok {
		return
	}
	id, ok := parsePaymentOrderID(r)
	if !ok {
		writeError(w, 404, "Status QRIS tidak ditemukan")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var status, midtransStatus string
	var orderID, transactionID sql.NullString
	var amount int64
	err := api.pool.QueryRow(ctx, `SELECT status,midtrans_status,midtrans_order_id,midtrans_transaction_id,total_idr FROM payment_orders WHERE id=$1 AND owner_id=$2 AND payment_method='QRIS' AND midtrans_order_id IS NOT NULL`, id, ownerID).Scan(&status, &midtransStatus, &orderID, &transactionID, &amount)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "Status QRIS tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, 503, "Status QRIS tidak tersedia")
		return
	}
	qrURL := any(nil)
	if transactionID.Valid && transactionID.String != "" {
		qrURL = api.midtransBase() + "/v2/qris/" + url.PathEscape(transactionID.String) + "/qr-code"
	}
	writeJSON(w, 200, map[string]any{"order_id": id, "status": status, "midtrans_status": midtransStatus, "midtrans_order_id": nullableString(orderID), "qr_code_url": qrURL, "gross_amount": amount})
}

func (api paymentAPI) syncQRIS(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := api.client(w, r)
	if !ok {
		return
	}
	id, ok := parsePaymentOrderID(r)
	if !ok {
		writeError(w, 404, "Pesanan tidak ditemukan")
		return
	}
	if api.midtransServerKey == "" {
		writeError(w, 503, "Pembayaran QRIS belum tersedia.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	var midtransID string
	var currentStatus, midtransStatus string
	var currentAccount sql.NullInt64
	err := api.pool.QueryRow(ctx, `SELECT midtrans_order_id,status,midtrans_status,account_id FROM payment_orders WHERE id=$1 AND owner_id=$2 AND payment_method='QRIS'`, id, ownerID).Scan(&midtransID, &currentStatus, &midtransStatus, &currentAccount)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "Pesanan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, 503, "Status QRIS tidak tersedia")
		return
	}
	if currentStatus != "PENDING_PAYMENT" {
		writeJSON(w, 200, map[string]any{"order_id": id, "order_status": currentStatus, "midtrans_status": midtransStatus, "result": currentStatus, "account_id": nullableInt(currentAccount)})
		return
	}
	data, code, err := api.midtransRequest(ctx, http.MethodGet, "/v2/"+url.PathEscape(midtransID)+"/status", nil)
	if err != nil {
		writeError(w, 502, "Tidak dapat menghubungi Midtrans. Coba lagi sebentar.")
		return
	}
	if code == http.StatusNotFound {
		_, err = api.pool.Exec(ctx, `UPDATE payment_orders SET payment_method='MANUAL',midtrans_order_id=NULL,midtrans_transaction_id=NULL,midtrans_status='expire',updated_at=$2 WHERE id=$1 AND status='PENDING_PAYMENT'`, id, time.Now().UTC())
		if err != nil {
			writeError(w, 503, "Status QRIS tidak dapat disimpan")
			return
		}
		writeJSON(w, 200, map[string]any{"order_id": id, "order_status": "PENDING_PAYMENT", "midtrans_status": "expire", "result": "PAYMENT_FAILED", "account_id": nil})
		return
	}
	if code != 200 {
		writeError(w, 502, "Midtrans belum dapat memeriksa status QRIS.")
		return
	}
	state, result, accountID, applyErr := api.applyMidtransStatus(ctx, id, data)
	if applyErr != nil {
		writeError(w, 503, "Status pembayaran tidak dapat disimpan")
		return
	}
	if result == "AMOUNT_MISMATCH" {
		data.TransactionStatus = midtransStatus
	}
	writeJSON(w, 200, map[string]any{"order_id": id, "order_status": state, "midtrans_status": data.TransactionStatus, "result": result, "account_id": accountID})
}

func (api paymentAPI) applyMidtransStatus(ctx context.Context, id int64, data midtransReply) (string, string, any, error) {
	tx, err := api.pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback(ctx)
	var status string
	var expected int64
	var plan string
	var owner int64
	var server, login string
	var mode string
	var existingAccount sql.NullInt64
	err = tx.QueryRow(ctx, `SELECT status,total_idr,plan,owner_id,server,login,execution_mode,account_id FROM payment_orders WHERE id=$1 FOR UPDATE`, id).Scan(&status, &expected, &plan, &owner, &server, &login, &mode, &existingAccount)
	if err != nil {
		return "", "", nil, err
	}
	amount, ok := parseMidtransAmount(data.GrossAmount)
	if !ok {
		return status, "AMOUNT_MISMATCH", nullableInt(existingAccount), nil
	}
	if amount != expected {
		return status, "AMOUNT_MISMATCH", nullableInt(existingAccount), nil
	}
	if _, err = tx.Exec(ctx, `UPDATE payment_orders SET midtrans_status=$2,midtrans_transaction_id=COALESCE(NULLIF($3,''),midtrans_transaction_id),updated_at=$4 WHERE id=$1`, id, data.TransactionStatus, data.TransactionID, time.Now().UTC()); err != nil {
		return "", "", nil, err
	}
	result := "PENDING"
	accountID := nullableInt(existingAccount)
	switch {
	case (data.TransactionStatus == "capture" || data.TransactionStatus == "settlement") && (data.FraudStatus == "accept" || data.FraudStatus == ""):
		if status != "PENDING_PAYMENT" {
			result = status
			break
		}
		price, valid := api.resolvePlan(plan)
		if !valid || price != expected {
			result = "INVALID_ORDER"
			break
		}
		managed := mode == "MANAGED"
		var duplicateID sql.NullInt64
		err = tx.QueryRow(ctx, `SELECT id FROM mt5_accounts WHERE lower(server)=lower($1) AND login=$2 AND ($4::boolean OR owner_id=$3) ORDER BY id LIMIT 1`, server, login, owner, managed).Scan(&duplicateID)
		if err == nil {
			result = "ACCOUNT_EXISTS"
			accountID = duplicateID.Int64
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil, err
		}
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return "", "", nil, err
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		hash := sha256.Sum256([]byte(token))
		var createdID int64
		err = tx.QueryRow(ctx, `INSERT INTO mt5_accounts(owner_id,label,server,login,plan,role,execution_mode,token_hash,active,created_at) SELECT owner_id,label,server,login,plan,'follower',execution_mode,$2,true,$3 FROM payment_orders WHERE id=$1 ON CONFLICT DO NOTHING RETURNING id`, id, hex.EncodeToString(hash[:]), time.Now().UTC()).Scan(&createdID)
		if errors.Is(err, pgx.ErrNoRows) {
			result = "ACCOUNT_EXISTS"
			break
		}
		if err != nil {
			return "", "", nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE payment_orders SET status='APPROVED',account_id=$2,updated_at=$3 WHERE id=$1`, id, createdID, time.Now().UTC()); err != nil {
			return "", "", nil, err
		}
		result = "APPROVED"
		accountID = createdID
	case data.TransactionStatus == "deny" || data.TransactionStatus == "cancel" || data.TransactionStatus == "expire" || data.TransactionStatus == "failure":
		if status == "PENDING_PAYMENT" {
			if _, err = tx.Exec(ctx, `UPDATE payment_orders SET payment_method='MANUAL',midtrans_order_id=NULL,midtrans_transaction_id=NULL WHERE id=$1`, id); err != nil {
				return "", "", nil, err
			}
		}
		result = "PAYMENT_FAILED"
	}
	if err = tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	var finalStatus string
	_ = api.pool.QueryRow(ctx, `SELECT status FROM payment_orders WHERE id=$1`, id).Scan(&finalStatus)
	return finalStatus, result, accountID, nil
}

func parseMidtransAmount(raw string) (int64, bool) {
	rat, ok := new(big.Rat).SetString(raw)
	if !ok {
		return 0, false
	}
	// Python int(Decimal(...)) truncates fractional IDR toward zero.
	n := new(big.Int).Quo(rat.Num(), rat.Denom())
	if !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}
