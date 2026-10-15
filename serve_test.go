package maago

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// slow is a handler that tells it has started, then answers when
// released.
func slow(started chan<- struct{}, release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		io.WriteString(w, "done")
	})
}

// listen is a listener on a free local port.
func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// A stop waits for the request that is running, and takes no new
// connection (D-153).
func TestServeStop(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	started, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	stopped := false
	served := make(chan error, 1)
	go func() { served <- serveOn(ctx, func() { stopped = true }, ln, slow(started, release), time.Minute) }()
	got := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			got <- err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		got <- string(b)
	}()
	<-started
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for { // Shutdown closes the listener first
		c, err := net.Dial("tcp", addr)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("still taking connections after the stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if body := <-got; body != "done" {
		t.Errorf("the running request: %q", body)
	}
	if err := <-served; err != nil || !stopped {
		t.Errorf("serve: %v, stopped %t", err, stopped)
	}
}

// A request that outlasts the wait is cut, and serve says so.
func TestServeStopWaitEnds(t *testing.T) {
	ln := listen(t)
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serveOn(ctx, func() {}, ln, slow(started, release), 50*time.Millisecond) }()
	go http.Get("http://" + ln.Addr().String() + "/")
	<-started
	cancel()
	if err := <-served; err == nil || !strings.Contains(err.Error(), "1 requests were still running after 50ms") {
		t.Errorf("serve: %v", err)
	}
}

// SIGTERM stops Serve as a stop does: the running request finishes and
// the process exits cleanly. The server runs in a child process, which
// the signal is for.
func TestServeSignal(t *testing.T) {
	if addr := os.Getenv("MAAGO_TEST_SERVE"); addr != "" {
		err := Serve(context.Background(), addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			io.WriteString(w, "done")
		}))
		if err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if runtime.GOOS == "windows" {
		t.Skip("no SIGTERM")
	}
	ln := listen(t)
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestServeSignal$")
	cmd.Env = append(os.Environ(), "MAAGO_TEST_SERVE="+addr)
	var log strings.Builder
	cmd.Stderr = &log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	deadline := time.Now().Add(10 * time.Second)
	for { // the child is listening
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the child never listened")
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := make(chan string, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			got <- err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		got <- string(b)
	}()
	time.Sleep(100 * time.Millisecond) // the request is running
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if body := <-got; body != "done" {
		t.Errorf("the running request: %q", body)
	}
	if err := cmd.Wait(); err != nil || !strings.Contains(log.String(), "maago: shutting down, waiting up to 30s for 1 requests") {
		t.Errorf("the child: %v\n%s", err, log.String())
	}
}
