package maagotest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/maago-web/maago/contrib/auth"
)

func TestMailbox(t *testing.T) {
	unset := auth.Mail(context.Background(), auth.Message{})
	t.Run("sends", func(t *testing.T) {
		box := Mailbox(t)
		var wg sync.WaitGroup
		for i := range 10 { // as requests do, at once
			wg.Go(func() { auth.Mail(context.Background(), auth.Message{To: fmt.Sprintf("u%d@example.com", i)}) })
		}
		wg.Wait()
		got := box.Messages()
		if len(got) != 10 {
			t.Errorf("got %d messages", len(got))
		}
		for i := range 10 {
			if !slices.Contains(got, auth.Message{To: fmt.Sprintf("u%d@example.com", i)}) {
				t.Errorf("no mail to u%d in %v", i, got)
			}
		}
	})
	if err := auth.Mail(context.Background(), auth.Message{}); err == nil || err.Error() != unset.Error() {
		t.Errorf("after the test, Mail is not what it was: %v", err)
	}
}
