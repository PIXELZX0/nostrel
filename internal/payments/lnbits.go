package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LNbits speaks the LNbits REST API. Field names differ slightly between LNbits
// versions (payment_request vs bolt11, paid vs status), so both spellings are
// accepted on the way in.
type LNbits struct {
	baseURL string
	key     string
	webhook string
	http    *http.Client
}

// NewLNbits configures the client. webhook is where LNbits should announce
// settled invoices; it only speeds things up, the poller settles them anyway if
// the notification never arrives (or the panel isn't reachable from LNbits).
func NewLNbits(baseURL, invoiceKey, webhook string) *LNbits {
	return &LNbits{
		baseURL: baseURL,
		key:     invoiceKey,
		webhook: webhook,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (l *LNbits) Name() string { return "lnbits" }

func (l *LNbits) CreateInvoice(ctx context.Context, sats int64, memo string) (*Invoice, error) {
	request := map[string]any{
		"out":    false,
		"amount": sats,
		"unit":   "sat",
		"memo":   memo,
	}
	if l.webhook != "" {
		request["webhook"] = l.webhook
	}
	body, _ := json.Marshal(request)

	var out struct {
		PaymentHash    string `json:"payment_hash"`
		CheckingID     string `json:"checking_id"`
		PaymentRequest string `json:"payment_request"`
		Bolt11         string `json:"bolt11"`
	}
	if err := l.do(ctx, http.MethodPost, "/api/v1/payments", body, &out); err != nil {
		return nil, err
	}

	inv := &Invoice{PaymentHash: out.PaymentHash, Bolt11: out.PaymentRequest}
	if inv.PaymentHash == "" {
		inv.PaymentHash = out.CheckingID
	}
	if inv.Bolt11 == "" {
		inv.Bolt11 = out.Bolt11
	}
	if inv.PaymentHash == "" || inv.Bolt11 == "" {
		return nil, fmt.Errorf("lnbits: invoice response missing payment_hash or bolt11")
	}
	return inv, nil
}

// Check verifies the URL and key by reading the wallet they belong to, without
// creating anything.
func (l *LNbits) Check(ctx context.Context) (string, error) {
	var wallet struct {
		Name    string `json:"name"`
		Balance int64  `json:"balance"` // millisats
	}
	if err := l.do(ctx, http.MethodGet, "/api/v1/wallet", nil, &wallet); err != nil {
		return "", err
	}
	if wallet.Name == "" {
		wallet.Name = "wallet"
	}
	return fmt.Sprintf("connected to LNbits %q, balance %d sats", wallet.Name, wallet.Balance/1000), nil
}

func (l *LNbits) IsPaid(ctx context.Context, paymentHash string) (bool, error) {
	var out struct {
		Paid   *bool  `json:"paid"`
		Status string `json:"status"`
	}
	if err := l.do(ctx, http.MethodGet, "/api/v1/payments/"+paymentHash, nil, &out); err != nil {
		return false, err
	}
	if out.Paid != nil {
		return *out.Paid, nil
	}
	return out.Status == "success", nil
}

func (l *LNbits) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", l.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := l.http.Do(req)
	if err != nil {
		return fmt.Errorf("lnbits %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("lnbits %s %s: status %d: %s", method, path, resp.StatusCode, truncate(payload, 200))
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("lnbits %s %s: bad json: %w", method, path, err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
