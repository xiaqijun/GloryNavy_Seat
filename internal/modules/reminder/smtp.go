package reminder

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// smtpConfig never appears in API responses or log messages.
type smtpConfig struct {
	Host, Username, Password, From string
	Port                           int
	ImplicitTLS                    bool
	rootCAs                        *x509.CertPool
}

func (s *Service) sendEmail(ctx context.Context, cfg smtpConfig, recipient, subject, body string) Result {
	if s.emailSend != nil {
		return s.emailSend(ctx, cfg, recipient, subject, body)
	}
	return smtpSend(ctx, cfg, recipient, subject, body)
}

func smtpMailbox(value string) (string, error) {
	if value == "" || len(value) > 254 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid mailbox")
	}
	a, err := mail.ParseAddress(value)
	if err != nil || a.Name != "" || a.Address != value || strings.Count(value, "@") != 1 {
		return "", errors.New("invalid mailbox")
	}
	parts := strings.Split(value, "@")
	if !strings.Contains(parts[1], ".") || strings.ContainsAny(value, " <>\t,;") {
		return "", errors.New("invalid mailbox")
	}
	for _, c := range value {
		if c < 33 || c > 126 {
			return "", errors.New("invalid mailbox")
		}
	}
	return value, nil
}

func smtpMessage(from, to, subject, body string) ([]byte, error) {
	if _, err := smtpMailbox(from); err != nil {
		return nil, err
	}
	if _, err := smtpMailbox(to); err != nil {
		return nil, err
	}
	if subject == "" || len(subject) > 512 || strings.ContainsAny(subject, "\r\n\x00") {
		return nil, errors.New("invalid subject")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var content strings.Builder
	fmt.Fprintf(&content, "Date: %s\r\nFrom: <%s>\r\nTo: <%s>\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", time.Now().Format(time.RFC1123Z), from, to, mime.QEncoding.Encode("utf-8", subject))
	for len(encoded) > 76 {
		content.WriteString(encoded[:76])
		content.WriteString("\r\n")
		encoded = encoded[76:]
	}
	content.WriteString(encoded)
	content.WriteString("\r\n")
	return []byte(content.String()), nil
}

// smtpSend returns unknown when the server may have accepted DATA but the
// final response is unavailable. That result must not be retried implicitly.
func smtpSend(ctx context.Context, cfg smtpConfig, recipient, subject, body string) Result {
	fail := Result{State: "failed", Reason: "service_unavailable"}
	message, err := smtpMessage(cfg.From, recipient, subject, body)
	if err != nil || cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || strings.ContainsAny(cfg.Host, "/\\\r\n\t ") || cfg.Username == "" || cfg.Password == "" {
		return fail
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		return fail
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12, RootCAs: cfg.rootCAs}
	if cfg.ImplicitTLS {
		secure := tls.Client(conn, tlsCfg)
		if err = secure.HandshakeContext(ctx); err != nil {
			return fail
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fail
	}
	defer client.Close()
	if !cfg.ImplicitTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fail
		}
		if err = client.StartTLS(tlsCfg); err != nil {
			return fail
		}
	}
	if err = client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
		return fail
	}
	if err = client.Mail(cfg.From); err != nil {
		return fail
	}
	if err = client.Rcpt(recipient); err != nil {
		return fail
	}
	data, err := client.Data()
	if err != nil {
		return fail
	}
	if _, err = data.Write(message); err != nil {
		return fail
	}
	if err = data.Close(); err != nil {
		var smtpErr *textproto.Error
		if errors.As(err, &smtpErr) && smtpErr.Code >= 400 && smtpErr.Code < 600 {
			return Result{State: "failed", Reason: "mail_rejected"}
		}
		return Result{State: "unknown", Reason: "confirmation_required"}
	}
	return Result{State: "sent"}
}
