package payments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"sync"
	"time"
)

// Mock is a fake lightning backend for local development and tests: invoices
// settle by themselves after MOCK_PAY_DELAY seconds (default 0).
type Mock struct {
	mu       sync.Mutex
	issuedAt map[string]time.Time
	delay    time.Duration
}

func NewMock() *Mock {
	delay := 0
	if v := os.Getenv("MOCK_PAY_DELAY"); v != "" {
		delay, _ = strconv.Atoi(v)
	}
	return &Mock{issuedAt: map[string]time.Time{}, delay: time.Duration(delay) * time.Second}
}

func (m *Mock) Name() string { return "mock" }

// Check exists so the panel's connection test works for every backend.
func (m *Mock) Check(ctx context.Context) (string, error) {
	return "mock backend: invoices settle themselves, never use this in production", nil
}

func (m *Mock) CreateInvoice(ctx context.Context, sats int64, memo string) (*Invoice, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	hash := hex.EncodeToString(buf)

	m.mu.Lock()
	m.issuedAt[hash] = time.Now()
	m.mu.Unlock()

	return &Invoice{PaymentHash: hash, Bolt11: "lnbcmock1" + hash}, nil
}

func (m *Mock) IsPaid(ctx context.Context, paymentHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	issued, ok := m.issuedAt[paymentHash]
	if !ok {
		return false, nil
	}
	return time.Since(issued) >= m.delay, nil
}
