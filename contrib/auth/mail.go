package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
	"unicode/utf8"
)

// Message is one mail: plain text, to one address.
type Message struct {
	To      string // ann@example.com, or Ann <ann@example.com>
	Subject string
	Body    string
}

// Mail sends m. main sets it, to SMTP(…) or ConsoleMail; tests
// use maagotest.Mailbox. Until then it fails: a mail is never dropped.
var Mail = func(ctx context.Context, m Message) error {
	return errors.New("auth: Mail is not set; main sets auth.Mail to auth.SMTP(addr, from, a) or auth.ConsoleMail")
}

// mailTimeout bounds a whole SMTP exchange: net/smtp has no timeout, and
// a request's context seldom ends.
const mailTimeout = 30 * time.Second

// SMTP sends through the server at addr, host:port, from the address
// from, signing in with a when it is not nil: smtp.PlainAuth(…) mostly.
// It uses STARTTLS when the server offers it, and sends a only over TLS
// or to this machine.
//
// ponytail: STARTTLS only, no implicit TLS (port 465); port 587 serves
// every provider. No Message-ID: the first server adds one.
func SMTP(addr, from string, a smtp.Auth) func(context.Context, Message) error {
	return func(ctx context.Context, m Message) error {
		msg, sender, rcpt, err := compose(from, m)
		if err != nil {
			return err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("auth: SMTP address: %w", err)
		}
		deadline := time.Now().Add(mailTimeout)
		conn, err := (&net.Dialer{Deadline: deadline}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		defer conn.Close()
		conn.SetDeadline(deadline)
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		err = send(conn, host, a, sender, rcpt, msg)
		if !stop() && err != nil {
			return ctx.Err() // the context closed the connection: that is the error
		}
		return err
	}
}

// send is smtp.SendMail on conn.
func send(conn net.Conn, host string, a smtp.Auth, from, to string, msg []byte) error {
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	if err := c.Hello("localhost"); err != nil {
		return err
	}
	secure := false
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
		secure = true
	}
	if a != nil {
		if ip := net.ParseIP(host); !secure && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("auth: the SMTP server offers no STARTTLS; the password is not sent in the clear")
		}
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("auth: the SMTP server does not support AUTH")
		}
		if err := c.Auth(a); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data() // its writer turns lines into CRLF and escapes a leading dot
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// maxHeader keeps each header within one line of SMTP, 1000 bytes: they
// are not folded.
const maxHeader = 900

// compose writes m as a mail from from, and returns the addresses of the
// envelope. Quoted-printable needs no 8BITMIME of the server.
func compose(from string, m Message) (msg []byte, sender, rcpt string, err error) {
	f, err := address(from)
	if err != nil {
		return nil, "", "", err
	}
	t, err := address(m.To)
	if err != nil {
		return nil, "", "", err
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, "", "", errors.New("auth: a mail subject is one line")
	}
	subject := mime.QEncoding.Encode("utf-8", m.Subject)
	for _, h := range []string{f.String(), t.String(), subject} {
		if len(h) > maxHeader {
			return nil, "", "", fmt.Errorf("auth: a mail header is longer than %d bytes", maxHeader)
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", f, t, subject, time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	qp.Write([]byte(m.Body))
	qp.Close()
	return b.Bytes(), f.Address, t.Address, nil
}

// address reads one address, whose part after the name is ASCII: an
// SMTP server need not take more.
func address(s string) (*mail.Address, error) {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return nil, fmt.Errorf("auth: mail address %q: %w", s, err)
	}
	if strings.ContainsFunc(a.Address, func(r rune) bool { return r >= utf8.RuneSelf }) {
		return nil, fmt.Errorf("auth: mail address %q is not ASCII", s)
	}
	return a, nil
}

// ConsoleMail prints m instead of sending it, for development.
func ConsoleMail(ctx context.Context, m Message) error {
	_, err := fmt.Printf("To: %s\nSubject: %s\n\n%s\n\n", m.To, m.Subject, m.Body)
	return err
}
