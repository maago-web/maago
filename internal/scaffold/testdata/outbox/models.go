package outbox

import (
	"time"

	"github.com/maago-web/maago"
)

// Job is work to do after a commit, such as a webhook to call. A worker
// takes it when RunAt has passed: then RunAt moves on by the lease, and
// Attempts by one, so a worker that dies with it leaves it to be taken
// again. A job that is done is deleted; one whose last attempt failed
// stays, with its LastError: to send it again, set its Attempts to 0 in
// the admin (with outbox.Admin in its Models), and it goes once RunAt
// has passed.
type Job struct {
	maago.Model `maago:"index=(RunAt),admin_list=(Kind,Attempts,RunAt,LastError,CreatedAt)"`

	ID        int64
	Kind      string    `maago:"max=100"` // what to do; handle tells kinds apart
	Payload   string    // JSON
	Attempts  int64     `maago:"default=0"`
	RunAt     time.Time // a worker may take it from then on
	LastError string    `maago:"default=''"`
	CreatedAt time.Time `maago:"auto=create"`
}
