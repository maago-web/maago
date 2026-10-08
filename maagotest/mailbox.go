package maagotest

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/maago-web/maago/contrib/auth"
)

// Inbox holds the mail sent while a test runs.
type Inbox struct {
	mu   sync.Mutex
	sent []auth.Message
}

// Mailbox makes auth.Mail put mail in a new Inbox until t ends, instead
// of sending it (D-126). It replaces a package variable, so a test that
// calls t.Parallel does not use it.
func Mailbox(t testing.TB) *Inbox {
	b := &Inbox{}
	old := auth.Mail
	auth.Mail = func(ctx context.Context, m auth.Message) error {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.sent = append(b.sent, m)
		return nil
	}
	t.Cleanup(func() { auth.Mail = old })
	return b
}

// Messages are the mail sent so far, oldest first.
func (b *Inbox) Messages() []auth.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.sent)
}
