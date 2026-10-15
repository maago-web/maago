package maago

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// shutdownWait is how long Serve waits for the requests still running
// when it stops: as long as a mail of auth may take.
const shutdownWait = 30 * time.Second

// Serve serves h on addr until ctx ends or the process gets SIGINT or
// SIGTERM (D-153). Then it stops taking connections and waits up to 30
// seconds for the requests that are running, the work they gave
// AfterCommit included; a second signal kills the process at once. It
// returns nil after a stop in which every request finished. A request may
// take its headers 10 seconds, and an idle connection is closed after 2
// minutes. There is no limit on reading a body or writing a response,
// which would cut a slow upload or a stream, or lose the response of a
// view that has committed: a proxy in front is what holds slow clients.
// A server that needs other limits, or TLS, is an http.Server of main's.
func Serve(ctx context.Context, addr string, h http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveOn(ctx, stop, ln, h, shutdownWait)
}

// serveOn is Serve on ln, waiting up to wait.
func serveOn(ctx context.Context, stop func(), ln net.Listener, h http.Handler, wait time.Duration) error {
	var running atomic.Int64
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			running.Add(1)
			defer running.Add(-1)
			h.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	stop() // a second signal kills
	log.Printf("maago: shutting down, waiting up to %s for %d requests", wait, running.Load())
	sctx, cancel := context.WithTimeout(context.Background(), wait) // ctx has ended
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		n := running.Load()
		srv.Close()
		return fmt.Errorf("maago: %d requests were still running after %s, and were cut", n, wait)
	}
	return srv.Close() // the hijacked connections, which Shutdown leaves
}
