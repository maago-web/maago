// Package outbox is work that must happen once a transaction commits,
// even if the process stops: a webhook to call, a mail to send.
// "maago add outbox" wrote it, and it is yours now.
//
// A view writes a job in its transaction, with Enqueue: if the
// transaction rolls back, there is no job. The payload goes as JSON:
//
//	if _, err := outbox.Enqueue(ctx, tx, "post_created", p); err != nil {
//		return err
//	}
//
// A worker in main hands the jobs to a function of yours, beside the
// server; it reads the payload back with json.Unmarshal([]byte(j.Payload), &p):
//
//	// … open db, make the handler h …
//	work, stop := context.WithCancel(ctx)
//	worked := make(chan struct{})
//	go func() {
//		outbox.Run(work, db, deliver) // deliver(ctx, j) does what j.Kind says
//		close(worked)
//	}()
//	err = maago.Serve(ctx, ":8000", h)
//	stop()
//	<-worked // the job in hand is finished
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Once Serve has stopped, the worker takes no more jobs and finishes the
// one in hand, for up to the lease (5 minutes): give the work in deliver
// a shorter deadline of its own. A second signal kills the process at
// once; the job goes again later.
//
// A job goes at least once, not exactly once: a worker that dies, or
// takes longer than the lease, leaves it to be done again. Send j.ID
// along (as an Idempotency-Key header, say), so that the receiver can
// drop a repeat. When deliver returns an error, or panics, the job
// waits, 2 seconds, then twice as long each time, and goes again; after
// 12 attempts it stays, with its LastError: the admin shows the jobs
// once main gives it outbox.Admin, in admin.Config's Models.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/maago-web/maago"
)

const (
	maxAttempts = 12              // then a job stays, for the admin to show
	lease       = 5 * time.Minute // a taken job is its worker's this long; handle's ctx ends then
	poll        = 5 * time.Second // how often an idle worker looks again
)

// wait is how long a job waits after its n-th attempt failed: 2
// seconds, 4, … up to about 68 minutes.
func wait(n int64) time.Duration { return time.Second << min(n, 12) }

// Enqueue writes a job of kind with payload as JSON, due now, in the
// transaction tx: if it rolls back, there is no job.
func Enqueue(ctx context.Context, tx maago.Tx, kind string, payload any) (Job, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Job{}, err
	}
	return InsertJob(ctx, tx, NewJobInput{Kind: kind, Payload: string(b), RunAt: time.Now()})
}

// Run hands the jobs to handle, one at a time, until ctx ends; then it
// finishes the job in hand and returns. A database error is logged, and
// Run looks again a while later.
func Run(ctx context.Context, db maago.DB, handle func(ctx context.Context, j Job) error) {
	for ctx.Err() == nil {
		found, err := RunOnce(ctx, db, time.Now(), handle)
		if err != nil && ctx.Err() == nil { // not the stop
			log.Printf("outbox: %v", err)
		}
		if found && err == nil {
			continue
		}
		select {
		case <-time.After(poll):
		case <-ctx.Done():
		}
	}
}

// RunOnce takes a job that is due at now, if there is one, and hands it
// to handle; found reports whether there was one. When handle returns
// nil, the job is deleted; when it returns an error or panics, the job
// waits and goes again, until its last attempt. handle's ctx goes on
// when ctx ends, and ends with the lease.
func RunOnce(ctx context.Context, db maago.DB, now time.Time, handle func(ctx context.Context, j Job) error) (found bool, err error) {
	start := time.Now() // the wait counts from the failure: now plus the time since
	j, err := ClaimJob(ctx, db, ClaimJobParams{Now: now, Until: now.Add(lease), Max: maxAttempts})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ctx = context.WithoutCancel(ctx) // a worker that stops finishes the job in hand
	herr := call(ctx, handle, j)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second) // and records it, even with the database stuck
	defer cancel()
	if herr != nil {
		log.Printf("outbox: job %d (%s), attempt %d: %v", j.ID, j.Kind, j.Attempts, herr)
		_, err = RetryJob(ctx, db, RetryJobParams{ID: j.ID, Attempts: j.Attempts, At: now.Add(time.Since(start) + wait(j.Attempts)), LastError: herr.Error()})
		return true, err
	}
	if err := DeleteJob(ctx, db, j.ID); err != nil && !errors.Is(err, sql.ErrNoRows) { // none: a worker that took it after the lease did it
		return true, err
	}
	return true, nil
}

// call is handle(ctx, j) until the lease ends, with a panic as its
// error.
func call(ctx context.Context, handle func(ctx context.Context, j Job) error, j Job) (err error) {
	ctx, cancel := context.WithTimeout(ctx, lease)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return handle(ctx, j)
}
