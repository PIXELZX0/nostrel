package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
)

// nwcTimeout bounds how long we wait for a wallet to answer a request.
const nwcTimeout = 30 * time.Second

// NWC talks to a lightning wallet over NIP-47 (Nostr Wallet Connect): requests
// are kind 23194 events encrypted to the wallet's pubkey, replies come back as
// kind 23195.
type NWC struct {
	walletPubkey string
	relayURL     string
	secret       string // our key, from the connection URI
	sharedSecret []byte

	mu    sync.Mutex
	relay *nostr.Relay
}

// NewNWC parses a nostr+walletconnect:// connection string.
func NewNWC(uri string) (*NWC, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("NWC_URI: %w", err)
	}
	if !strings.HasPrefix(uri, "nostr+walletconnect:") {
		return nil, errors.New("NWC_URI must start with nostr+walletconnect:")
	}

	walletPubkey := u.Host
	if walletPubkey == "" {
		walletPubkey = strings.TrimPrefix(u.Opaque, "//")
	}
	q := u.Query()
	relayURL, secret := q.Get("relay"), q.Get("secret")
	if walletPubkey == "" || relayURL == "" || secret == "" {
		return nil, errors.New("NWC_URI must contain a wallet pubkey, a relay and a secret")
	}

	shared, err := nip04.ComputeSharedSecret(walletPubkey, secret)
	if err != nil {
		return nil, fmt.Errorf("NWC_URI: %w", err)
	}

	return &NWC{
		walletPubkey: walletPubkey,
		relayURL:     relayURL,
		secret:       secret,
		sharedSecret: shared,
	}, nil
}

func (n *NWC) Name() string { return "nwc" }

func (n *NWC) CreateInvoice(ctx context.Context, sats int64, memo string) (*Invoice, error) {
	var result struct {
		Invoice     string `json:"invoice"`
		PaymentHash string `json:"payment_hash"`
	}
	if err := n.call(ctx, "make_invoice", map[string]any{
		"amount":      sats * 1000, // NIP-47 amounts are millisats
		"description": memo,
	}, &result); err != nil {
		return nil, err
	}
	if result.Invoice == "" || result.PaymentHash == "" {
		return nil, errors.New("nwc: wallet returned an incomplete invoice")
	}
	return &Invoice{PaymentHash: result.PaymentHash, Bolt11: result.Invoice}, nil
}

func (n *NWC) IsPaid(ctx context.Context, paymentHash string) (bool, error) {
	var result struct {
		SettledAt int64  `json:"settled_at"`
		Preimage  string `json:"preimage"`
	}
	err := n.call(ctx, "lookup_invoice", map[string]any{"payment_hash": paymentHash}, &result)
	if err != nil {
		// an unpaid invoice may legitimately be unknown to some wallets
		if strings.Contains(err.Error(), "NOT_FOUND") {
			return false, nil
		}
		return false, err
	}
	return result.SettledAt > 0 || result.Preimage != "", nil
}

// Check asks the wallet who it is, which exercises the relay connection, the
// shared secret and the wallet's permissions in one round trip.
func (n *NWC) Check(ctx context.Context) (string, error) {
	var info struct {
		Alias   string   `json:"alias"`
		Methods []string `json:"methods"`
	}
	if err := n.call(ctx, "get_info", map[string]any{}, &info); err != nil {
		return "", err
	}
	if info.Alias == "" {
		info.Alias = "wallet"
	}
	for _, required := range []string{"make_invoice", "lookup_invoice"} {
		if len(info.Methods) > 0 && !slices.Contains(info.Methods, required) {
			return "", fmt.Errorf("connected to %q but it does not allow %s", info.Alias, required)
		}
	}
	return fmt.Sprintf("connected to %q over NWC", info.Alias), nil
}

// call performs one NIP-47 request/response round trip.
func (n *NWC) call(ctx context.Context, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, nwcTimeout)
	defer cancel()

	relay, err := n.conn(ctx)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		return err
	}
	content, err := nip04.Encrypt(string(payload), n.sharedSecret)
	if err != nil {
		return err
	}

	req := nostr.Event{
		Kind:      nostr.KindNWCWalletRequest,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"p", n.walletPubkey}},
		Content:   content,
	}
	if err := req.Sign(n.secret); err != nil {
		return err
	}

	// subscribe before publishing so a fast wallet can't answer into the void
	sub, err := relay.Subscribe(ctx, nostr.Filters{{
		Kinds: []int{nostr.KindNWCWalletResponse},
		Tags:  nostr.TagMap{"e": []string{req.ID}},
		Limit: 1,
	}})
	if err != nil {
		return fmt.Errorf("nwc: subscribing: %w", err)
	}
	defer sub.Unsub()

	if err := relay.Publish(ctx, req); err != nil {
		n.reset()
		return fmt.Errorf("nwc: publishing %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("nwc: wallet did not answer %s in time", method)
	case evt := <-sub.Events:
		if evt == nil {
			return fmt.Errorf("nwc: subscription closed before %s was answered", method)
		}
		plain, err := nip04.Decrypt(evt.Content, n.sharedSecret)
		if err != nil {
			return fmt.Errorf("nwc: decrypting response: %w", err)
		}
		var resp struct {
			ResultType string          `json:"result_type"`
			Result     json.RawMessage `json:"result"`
			Error      *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(plain), &resp); err != nil {
			return fmt.Errorf("nwc: bad response json: %w", err)
		}
		if resp.Error != nil {
			return fmt.Errorf("nwc: %s failed: %s %s", method, resp.Error.Code, resp.Error.Message)
		}
		return json.Unmarshal(resp.Result, result)
	}
}

// conn returns a live relay connection, dialling on first use and after a drop.
func (n *NWC) conn(ctx context.Context) (*nostr.Relay, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.relay != nil && n.relay.IsConnected() {
		return n.relay, nil
	}
	relay, err := nostr.RelayConnect(ctx, n.relayURL)
	if err != nil {
		return nil, fmt.Errorf("nwc: connecting to %s: %w", n.relayURL, err)
	}
	n.relay = relay
	return relay, nil
}

func (n *NWC) reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.relay != nil {
		n.relay.Close()
		n.relay = nil
	}
}
