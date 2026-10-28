package outbox

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/maagotest"
)

// job writes a job that is due, and returns it as the database has it.
func job(t *testing.T, db maago.DB) Job {
	t.Helper()
	j, err := InsertJob(t.Context(), db, NewJobInput{Kind: "note", Payload: `"x"`, RunAt: time.Now()})
	if err == nil {
		j, err = GetJob(t.Context(), db, j.ID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func fail(context.Context, Job) error { return errors.New("down") }

// A job written in a transaction that rolls back is not there; one that
// commits is handed to handle once, then deleted.
func TestEnqueue(t *testing.T) {
	db := maagotest.DB(t)
	err := maago.WithTx(t.Context(), db, func(ctx context.Context, tx maago.Tx) error {
		if _, err := Enqueue(ctx, tx, "note", "lost"); err != nil {
			return err
		}
		return errors.New("rolled back")
	})
	if err == nil {
		t.Fatal("no error")
	}
	if found, err := RunOnce(t.Context(), db, time.Now(), fail); found || err != nil {
		t.Fatalf("a job of a rollback: %v %v", found, err)
	}
	var j Job
	if err := maago.WithTx(t.Context(), db, func(ctx context.Context, tx maago.Tx) (err error) {
		j, err = Enqueue(ctx, tx, "note", map[string]int{"id": 7})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var got []Job
	found, err := RunOnce(t.Context(), db, time.Now(), func(ctx context.Context, j Job) error {
		got = append(got, j)
		return nil
	})
	if !found || err != nil || len(got) != 1 || got[0].ID != j.ID || got[0].Kind != "note" || got[0].Payload != `{"id":7}` || got[0].Attempts != 1 {
		t.Fatalf("%v %v %+v", found, err, got)
	}
	if _, err := GetJob(t.Context(), db, j.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the job is still there: %v", err)
	}
	if found, err := RunOnce(t.Context(), db, time.Now(), fail); found || err != nil {
		t.Errorf("again: %v %v", found, err)
	}
}

// A worker that took a job and died leaves it taken until its lease is
// up; then another takes it. Were the first only late, its failure
// would not put off the job the second has, and its success, having
// deleted the job, is no error to the second.
func TestDeadWorker(t *testing.T) {
	db := maagotest.DB(t)
	j := job(t, db)
	taken, err := ClaimJob(t.Context(), db, ClaimJobParams{Now: j.RunAt, Until: j.RunAt.Add(lease), Max: maxAttempts}) // and dies
	if err != nil {
		t.Fatal(err)
	}
	if found, err := RunOnce(t.Context(), db, taken.RunAt.Add(-time.Microsecond), fail); found || err != nil {
		t.Fatalf("before the lease is up: %v %v", found, err)
	}
	var got Job
	found, err := RunOnce(t.Context(), db, taken.RunAt, func(ctx context.Context, j Job) error {
		got = j
		late := RetryJobParams{ID: j.ID, Attempts: taken.Attempts, At: j.RunAt.Add(time.Hour), LastError: "late"}
		if n, err := RetryJob(ctx, db, late); n != 0 || err != nil {
			t.Errorf("the late worker's failure: %d rows, %v", n, err)
		}
		return DeleteJob(ctx, db, j.ID) // the late worker's success
	})
	if !found || err != nil || got.ID != j.ID || got.Attempts != 2 {
		t.Fatalf("after: %v %v %+v", found, err, got)
	}
}

// Of two workers that pick one job at once, one takes it: the second
// waits for the first's lock, then gets no row.
func TestClaimRace(t *testing.T) {
	db := maagotest.DB(t)
	j := job(t, db)
	p := ClaimJobParams{Now: j.RunAt, Until: j.RunAt.Add(lease), Max: maxAttempts}
	type claim struct {
		err error
		at  time.Time
	}
	second := make(chan claim, 1)
	var committing time.Time
	err := maago.WithTx(t.Context(), db, func(ctx context.Context, tx maago.Tx) error {
		if _, err := ClaimJob(ctx, tx, p); err != nil {
			return err
		}
		go func() {
			_, err := ClaimJob(t.Context(), db, p)
			second <- claim{err, time.Now()}
		}()
		time.Sleep(100 * time.Millisecond) // the second waits for the lock
		committing = time.Now()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c := <-second
	if !errors.Is(c.err, sql.ErrNoRows) || c.at.Before(committing) {
		t.Errorf("the second claim: %v, %v before the commit", c.err, committing.Sub(c.at))
	}
}

// Workers at once take each job once.
func TestWorkers(t *testing.T) {
	db := maagotest.DB(t)
	const jobs, workers = 40, 4
	for range jobs {
		job(t, db)
	}
	var mu sync.Mutex
	seen := map[int64]int{}
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for misses := 0; misses < 10; {
				found, err := RunOnce(t.Context(), db, time.Now(), func(ctx context.Context, j Job) error {
					mu.Lock()
					seen[j.ID]++
					mu.Unlock()
					return nil
				})
				if err != nil {
					t.Error(err)
				}
				if !found {
					misses++
				}
			}
		})
	}
	wg.Wait()
	if len(seen) != jobs {
		t.Errorf("%d jobs handled, want %d", len(seen), jobs)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("job %d handled %d times", id, n)
		}
	}
}

// A job that fails waits longer each time, and after maxAttempts stays,
// with its last error, for the admin to show. A panic is a failure.
func TestAttempts(t *testing.T) {
	db := maagotest.DB(t)
	j := job(t, db)
	for n := int64(1); n <= maxAttempts; n++ {
		handle, took := fail, time.Duration(0)
		switch n {
		case 2:
			handle = func(context.Context, Job) error { panic("bad job") }
		case 3: // the wait counts from when handle gave up
			took = 200 * time.Millisecond
			handle = func(ctx context.Context, j Job) error { time.Sleep(took); return fail(ctx, j) }
		}
		if found, err := RunOnce(t.Context(), db, j.RunAt.Add(-time.Microsecond), handle); found || err != nil {
			t.Fatalf("attempt %d, before its time: %v %v", n, found, err)
		}
		if found, err := RunOnce(t.Context(), db, j.RunAt, handle); !found || err != nil {
			t.Fatalf("attempt %d: %v %v", n, found, err)
		}
		prev := j.RunAt
		var err error
		if j, err = GetJob(t.Context(), db, j.ID); err != nil {
			t.Fatal(err)
		}
		if w := j.RunAt.Sub(prev); j.Attempts != n || w < time.Second<<n+took || w > time.Second<<n+took+time.Second {
			t.Fatalf("attempt %d: %+v, waits %v", n, j, w)
		}
		if want := map[bool]string{true: "panic: bad job", false: "down"}[n == 2]; j.LastError != want {
			t.Errorf("attempt %d: %q", n, j.LastError)
		}
	}
	if found, err := RunOnce(t.Context(), db, j.RunAt.Add(24*time.Hour), fail); found || err != nil {
		t.Errorf("after the last attempt: %v %v", found, err)
	}
}

// A worker that stops finishes the job in hand, whose ctx goes on until
// the lease ends, and takes no other.
func TestStop(t *testing.T) {
	db := maagotest.DB(t)
	first, second := job(t, db), job(t, db)
	ctx, stop := context.WithCancel(t.Context())
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var handled []int64
	var herr error
	var left time.Duration
	go func() {
		Run(ctx, db, func(ctx context.Context, j Job) error {
			handled = append(handled, j.ID)
			if len(handled) == 1 {
				close(started)
				<-release
				herr = ctx.Err()
				if d, ok := ctx.Deadline(); ok {
					left = time.Until(d)
				}
			}
			return nil
		})
		close(done)
	}()
	<-started
	stop()
	select {
	case <-done:
		t.Fatal("Run returned with a job in hand")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-done
	if herr != nil || left < lease-time.Minute || left > lease || len(handled) != 1 || handled[0] != first.ID {
		t.Errorf("the job's ctx: %v, %v left; handled %v", herr, left, handled)
	}
	if _, err := GetJob(t.Context(), db, first.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the first is still there: %v", err)
	}
	if _, err := GetJob(t.Context(), db, second.ID); err != nil {
		t.Errorf("the second: %v", err)
	}
}
