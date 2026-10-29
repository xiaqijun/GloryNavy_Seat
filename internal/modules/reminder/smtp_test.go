package reminder

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSMTPMailboxRejectsDisplayNamesAndHeaderInjection(t *testing.T) {
	for _, value := range []string{"", "Name <user@example.com>", "user@example.com\r\nBcc: other@example.com", "user@example.com,other@example.com", "user@localhost"} {
		if _, err := smtpMailbox(value); err == nil {
			t.Errorf("accepted invalid mailbox %q", value)
		}
	}
	if got, err := smtpMailbox("User@example.com"); err != nil || got != "User@example.com" {
		t.Fatalf("valid mailbox: %q %v", got, err)
	}
}

func TestSMTPEncryptedDeliveryAndUnknownResult(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test smtp"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	pool := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool.AddCert(parsed)
	for _, tc := range []struct {
		name           string
		implicit, drop bool
		want           string
	}{{"TLS", true, false, "sent"}, {"STARTTLS", false, false, "sent"}, {"ambiguous", true, true, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, e := listener.Accept()
				if e != nil {
					done <- e
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				if tc.implicit {
					conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
				}
				reader := bufio.NewReader(conn)
				_, _ = fmt.Fprint(conn, "220 test\r\n")
				_, e = reader.ReadString('\n')
				if e != nil {
					done <- e
					return
				}
				if !tc.implicit {
					_, _ = fmt.Fprint(conn, "250-test\r\n250 STARTTLS\r\n")
					_, e = reader.ReadString('\n')
					if e != nil {
						done <- e
						return
					}
					_, _ = fmt.Fprint(conn, "220 ready\r\n")
					conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
					reader = bufio.NewReader(conn)
					_, e = reader.ReadString('\n')
					if e != nil {
						done <- e
						return
					}
				}
				_, _ = fmt.Fprint(conn, "250-test\r\n250 AUTH PLAIN\r\n")
				for i := 0; i < 3; i++ {
					line, readErr := reader.ReadString('\n')
					if readErr != nil {
						done <- readErr
						return
					}
					if i == 0 && !strings.HasPrefix(line, "AUTH PLAIN ") {
						done <- fmt.Errorf("missing AUTH")
						return
					}
					if i == 0 {
						_, _ = fmt.Fprint(conn, "235 authenticated\r\n")
					} else {
						_, _ = fmt.Fprint(conn, "250 ok\r\n")
					}
				}
				line, e := reader.ReadString('\n')
				if e != nil || strings.TrimSpace(line) != "DATA" {
					done <- fmt.Errorf("DATA missing")
					return
				}
				_, _ = fmt.Fprint(conn, "354 continue\r\n")
				for {
					line, e = reader.ReadString('\n')
					if e != nil {
						done <- e
						return
					}
					if line == ".\r\n" {
						break
					}
				}
				if !tc.drop {
					_, _ = fmt.Fprint(conn, "250 accepted\r\n")
				}
				done <- nil
			}()
			_, rawPort, _ := net.SplitHostPort(listener.Addr().String())
			port, _ := strconv.Atoi(rawPort)
			result := smtpSend(context.Background(), smtpConfig{Host: "127.0.0.1", Port: port, ImplicitTLS: tc.implicit, Username: "user", Password: "secret", From: "sender@example.com", rootCAs: pool}, "recipient@example.com", "提醒", "测试")
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if result.State != tc.want {
				t.Fatalf("got %#v want %s", result, tc.want)
			}
		})
	}
}

func TestSMTPRefusesUnencryptedAuthentication(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			commands <- ""
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = fmt.Fprint(conn, "220 test\r\n")
		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')
		_, _ = fmt.Fprint(conn, "250-test\r\n250 AUTH PLAIN\r\n")
		cmd, _ := reader.ReadString('\n')
		commands <- cmd
	}()
	_, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	result := smtpSend(context.Background(), smtpConfig{Host: "127.0.0.1", Port: port, Username: "user", Password: "secret", From: "sender@example.com"}, "recipient@example.com", "提醒", "测试")
	if result.State != "failed" {
		t.Fatalf("result = %#v", result)
	}
	if cmd := <-commands; strings.HasPrefix(strings.ToUpper(cmd), "AUTH ") {
		t.Fatalf("sent authentication over plaintext")
	}
}

func TestSMTPMessageEncodesUntrustedHeadersAndBody(t *testing.T) {
	msg, err := smtpMessage("sender@example.com", "user@example.com", "提醒\r\nBcc: bad@example.com", "<b>事项</b>")
	if err == nil || len(msg) != 0 {
		t.Fatalf("must reject header injection")
	}
	msg, err = smtpMessage("sender@example.com", "user@example.com", "待审批提醒", "<b>事项</b>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(msg), "Content-Transfer-Encoding: base64\r\n") || strings.Contains(string(msg), "<b>事项</b>") {
		t.Fatalf("unexpected message encoding: %s", msg)
	}
}
