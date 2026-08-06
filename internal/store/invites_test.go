package store

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInviteRejections(t *testing.T) {
	s := newStore(t)

	if _, err := s.ClaimInvite("nope", alice); !errors.Is(err, ErrInviteUnknown) {
		t.Errorf("unknown code: %v, want ErrInviteUnknown", err)
	}

	expired, err := s.CreateInvite(Invite{
		PeriodDays: 30, MaxUses: 1, ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimInvite(expired.Code, alice); !errors.Is(err, ErrInviteExpired) {
		t.Errorf("expired code: %v, want ErrInviteExpired", err)
	}

	once, err := s.CreateInvite(Invite{PeriodDays: 30, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimInvite(once.Code, alice); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// the same person cannot burn the code twice
	if _, err := s.ClaimInvite(once.Code, alice); !errors.Is(err, ErrInviteAlreadyUsed) {
		t.Errorf("second claim by the same key: %v, want ErrInviteAlreadyUsed", err)
	}
	// and it is spent for everybody else
	if _, err := s.ClaimInvite(once.Code, bob); !errors.Is(err, ErrInviteExhausted) {
		t.Errorf("claim after exhaustion: %v, want ErrInviteExhausted", err)
	}

	// codes are case insensitive
	if _, err := s.ClaimInvite(mixedCase(once.Code), bob); !errors.Is(err, ErrInviteExhausted) {
		t.Errorf("uppercase code was treated as a different one: %v", err)
	}
}

func mixedCase(code string) string {
	out := []rune(code)
	for i := 0; i < len(out); i += 2 {
		if out[i] >= 'a' && out[i] <= 'z' {
			out[i] -= 32
		}
	}
	return string(out)
}

func TestUnlimitedInvite(t *testing.T) {
	s := newStore(t)
	inv, err := s.CreateInvite(Invite{PeriodDays: 7, MaxUses: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, pubkey := range []string{alice, bob, "cc" + alice[2:]} {
		if _, err := s.ClaimInvite(inv.Code, pubkey); err != nil {
			t.Fatalf("claim by %s: %v", pubkey[:4], err)
		}
	}
	after, _ := s.Invite(inv.Code)
	if after.Used != 3 {
		t.Errorf("used = %d, want 3", after.Used)
	}
}

// The last seat of a limited code must go to exactly one person even when
// several claim it at the same moment.
func TestConcurrentClaimsCannotOversell(t *testing.T) {
	s := newStore(t)
	const seats = 3
	inv, err := s.CreateInvite(Invite{PeriodDays: 30, MaxUses: seats})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 12)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// a distinct pubkey each, so nothing is rejected as a duplicate
			pubkey := string(rune('a'+i%26)) + alice[1:]
			_, results[i] = s.ClaimInvite(inv.Code, pubkey)
		}(i)
	}
	wg.Wait()

	granted := 0
	for _, err := range results {
		switch {
		case err == nil:
			granted++
		case errors.Is(err, ErrInviteExhausted):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if granted != seats {
		t.Errorf("%d claims succeeded, want exactly %d", granted, seats)
	}

	after, _ := s.Invite(inv.Code)
	if after.Used != seats {
		t.Errorf("used = %d, want %d", after.Used, seats)
	}
}
