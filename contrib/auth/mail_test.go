package auth

import (
	"bufio"
	"context"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

// fakeSMTP answers one SMTP session on a new port, offering ext, and
// returns its address and, once the session ends, what the client said.
func fakeSMTP(t *testing.T, ext ...string) (string, <-chan string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	said := make(chan string, 1)
	go func() {
		var b strings.Builder
		defer func() { said <- b.String() }()
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		r := textproto.NewReader(bufio.NewReader(conn))
		line := func() (string, bool) {
			l, err := r.ReadLine()
			b.WriteString(l + "\n")
			return l, err == nil
		}
		io.WriteString(conn, "220 fake\r\n")
		for {
			l, ok := line()
			if !ok {
				return
			}
			switch verb, _, _ := strings.Cut(l, " "); strings.ToUpper(verb) {
			case "EHLO":
				io.WriteString(conn, "250-fake\r\n")
				for _, e := range ext {
					io.WriteString(conn, "250-"+e+"\r\n")
				}
				io.WriteString(conn, "250 HELP\r\n")
			case "AUTH":
				io.WriteString(conn, "235 ok\r\n")
			case "DATA":
				io.WriteString(conn, "354 go on\r\n")
				for l != "." {
					if l, ok = line(); !ok {
						return
					}
				}
				io.WriteString(conn, "250 queued\r\n")
			case "QUIT":
				io.WriteString(conn, "221 bye\r\n")
				return
			default:
				io.WriteString(conn, "250 ok\r\n")
			}
		}
	}()
	return l.Addr().String(), said
}

func wait(t *testing.T, said <-chan string) string {
	t.Helper()
	select {
	case s := <-said:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end")
		return ""
	}
}

func TestSMTP(t *testing.T) {
	addr, said := fakeSMTP(t)
	m := Message{To: "Ann <ann@example.com>", Subject: "가입을 확인하세요", Body: "Hello 앤\n.hidden line\nend\rhere"}
	if err := SMTP(addr, "Site <noreply@example.com>", nil)(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	got := wait(t, said)
	for _, w := range []string{
		"MAIL FROM:<noreply@example.com>\n",
		"RCPT TO:<ann@example.com>\n",
		"From: \"Site\" <noreply@example.com>\nTo: \"Ann\" <ann@example.com>\nSubject: =?utf-8?q?",
		"Content-Transfer-Encoding: quoted-printable\n\nHello =EC=95=A4\n..hidden line\nend\nhere\n.\n", // a lone CR ends a line too
		"QUIT\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("no %q in\n%s", w, got)
		}
	}
	_, data, _ := strings.Cut(got, "DATA\n")
	data, _, _ = strings.Cut(data, "\n.\n")
	msg, err := mail.ReadMessage(strings.NewReader(strings.ReplaceAll(data, "\n..", "\n.")))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	body, _ := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil || subject != m.Subject || string(body) != "Hello 앤\n.hidden line\nend\nhere" {
		t.Errorf("read back: %q %q %v", subject, body, err)
	}
}

func TestSMTPAuth(t *testing.T) {
	addr, said := fakeSMTP(t, "AUTH PLAIN")
	if err := SMTP(addr, "noreply@example.com", smtp.PlainAuth("", "u", "p", "127.0.0.1"))(context.Background(), Message{To: "ann@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, said); !strings.Contains(got, "AUTH PLAIN AHUAcA==\n") {
		t.Errorf("no AUTH in\n%s", got)
	}

	addr, said = fakeSMTP(t)
	if err := SMTP(addr, "noreply@example.com", smtp.PlainAuth("", "u", "p", "127.0.0.1"))(context.Background(), Message{To: "ann@example.com"}); err == nil || !strings.Contains(err.Error(), "does not support AUTH") {
		t.Errorf("no AUTH offered: %v", err)
	}
	wait(t, said)

	// To another machine without TLS, no mechanism sends the password.
	addr, said = fakeSMTP(t, "AUTH CRAM-MD5")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = send(conn, "mail.example.com", smtp.CRAMMD5Auth("u", "p"), "noreply@example.com", "ann@example.com", nil)
	if err == nil || !strings.Contains(err.Error(), "no STARTTLS") {
		t.Errorf("plain text to another host: %v", err)
	}
	conn.Close()
	if got := wait(t, said); strings.Contains(got, "AUTH") {
		t.Errorf("sent AUTH:\n%s", got)
	}
}

// What cannot go into a mail fails before a connection: to a closed
// port, a dial would fail otherwise.
func TestSMTPRefuses(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	for _, c := range []struct {
		from string
		m    Message
		want string
	}{
		{"noreply@example.com", Message{To: "ann@example.com", Subject: "hi\r\nBcc: eve@example.com"}, "one line"},
		{"noreply@example.com", Message{To: "ann@example.com", Subject: strings.Repeat("a", 1000)}, "longer than 900 bytes"},
		{"noreply@example.com", Message{To: "ann@example.com\r\nBcc: eve@example.com"}, "mail address"},
		{"noreply@example.com", Message{To: "ann, bob@example.com"}, "mail address"},
		{"noreply@example.com", Message{To: "앤@example.com"}, "not ASCII"},
		{"not an address", Message{To: "ann@example.com"}, "mail address"},
	} {
		if err := SMTP(addr, c.from, nil)(context.Background(), c.m); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%.40q: %v", c.m.To+c.m.Subject, err)
		}
	}
}

// A server that does not answer ends with the context.
func TestSMTPContext(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := SMTP(l.Addr().String(), "noreply@example.com", nil)(ctx, Message{To: "ann@example.com"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v", err)
	}
}

func TestMailUnset(t *testing.T) {
	if err := Mail(context.Background(), Message{}); err == nil || !strings.Contains(err.Error(), "main sets auth.Mail") {
		t.Errorf("got %v", err)
	}
}
