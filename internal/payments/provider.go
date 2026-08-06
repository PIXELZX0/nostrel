// Package payments talks to lightning backends. A Provider only needs to be
// able to hand out an invoice and to answer whether it was settled — every
// entitlement decision is made in the billing package from those two facts.
package payments

import (
	"context"
)

type Invoice struct {
	PaymentHash string
	Bolt11      string
}

type Provider interface {
	Name() string
	CreateInvoice(ctx context.Context, sats int64, memo string) (*Invoice, error)
	// IsPaid must query the backend, never a local cache: it is the only thing
	// standing between a forged webhook and free relay access.
	IsPaid(ctx context.Context, paymentHash string) (bool, error)
}
