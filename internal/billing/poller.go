package billing

import (
	"context"
	"time"
)

const (
	// InvoiceTTL is how long a pending invoice stays checkable before it is
	// written off as expired.
	InvoiceTTL   = time.Hour
	pollInterval = 30 * time.Second
)

// Poll keeps checking pending invoices until ctx is cancelled. It is the
// fallback for backends without webhooks (NWC) and for webhooks that never
// arrive.
func (s *Service) Poll(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce(ctx)
		}
	}
}

func (s *Service) pollOnce(ctx context.Context) {
	pending, err := s.store.PendingPayments(InvoiceTTL)
	if err != nil {
		s.log.Printf("poller: listing pending payments: %v", err)
		return
	}
	for _, p := range pending {
		if _, err := s.Settle(ctx, p.PaymentHash); err != nil {
			s.log.Printf("poller: settling %s: %v", p.PaymentHash, err)
		}
	}
	if n, err := s.store.ExpireStalePayments(InvoiceTTL); err != nil {
		s.log.Printf("poller: expiring stale invoices: %v", err)
	} else if n > 0 {
		s.log.Printf("poller: %d invoice(s) expired", n)
	}
	// lapsed NIP-05 names and abandoned checkouts go back on sale
	if n, err := s.store.PurgeNip05Names(); err != nil {
		s.log.Printf("poller: purging nip-05 names: %v", err)
	} else if n > 0 {
		s.log.Printf("poller: %d nip-05 name(s) released", n)
	}
}
