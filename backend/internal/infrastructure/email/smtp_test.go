package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"mime"
	"net"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestSMTPEncryptedDeliveryAndAcceptedDataDoesNotRetryOnQuitFailure(t *testing.T) {
	certServer := httptest.NewTLSServer(nil)
	certificate := certServer.TLS.Certificates[0]
	cert := certServer.Certificate()
	certServer.Close()
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			messages := make(chan string, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, e := listener.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				tlsCfg := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
				secure := mode == "tls"
				if secure {
					conn = tls.Server(conn, tlsCfg)
				}
				reader := bufio.NewReader(conn)
				write := func(s string) { _, _ = io.WriteString(conn, s) }
				write("220 local test SMTP\r\n")
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if secure {
							write("250-local\r\n250 AUTH PLAIN\r\n")
						} else {
							write("250-local\r\n250 STARTTLS\r\n")
						}
					case strings.HasPrefix(line, "STARTTLS"):
						write("220 begin TLS\r\n")
						conn = tls.Server(conn, tlsCfg)
						reader = bufio.NewReader(conn)
						secure = true
					case strings.HasPrefix(line, "AUTH"):
						if !secure {
							t.Error("credentials sent before TLS")
							return
						}
						write("235 authenticated\r\n")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						write("250 ok\r\n")
					case strings.HasPrefix(line, "DATA"):
						write("354 send data\r\n")
						body, e := textproto.NewReader(reader).ReadDotBytes()
						if e != nil {
							return
						}
						messages <- string(body)
						write("250 accepted\r\n")
					case strings.HasPrefix(line, "QUIT"):
						return // accepted DATA must stay successful
					default:
						write("500 unsupported\r\n")
					}
				}
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			client := New(Config{Host: host, Port: port, From: "慧投 <sender@example.invalid>", Username: "fixture", Password: "fixture-password", TLSMode: mode})
			client.roots = x509.NewCertPool()
			client.roots.AddCert(cert)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Send(ctx, "receiver@example.invalid", "慧投提醒\r\nBcc: injected", "目标价已到达 90 USDT", "alert-1-1"); err != nil {
				t.Fatal(err)
			}
			raw := <-messages
			message, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if message.Header.Get("Bcc") != "" {
				t.Fatal("mail header injection")
			}
			subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
			if err != nil || !strings.Contains(subject, "慧投提醒") {
				t.Fatal("subject encoding lost")
			}
			body, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, message.Body))
			if string(body) != "目标价已到达 90 USDT" {
				t.Fatal("body encoding lost")
			}
			<-done
		})
	}
}
func TestSMTPRejectsMissingConfigAndPlaintextModes(t *testing.T) {
	for _, cfg := range []Config{{}, {Host: "example.invalid", Port: "25", From: "sender@example.invalid", TLSMode: "none"}, {Host: "example.invalid", Port: "587", From: "bad\r\nBcc:evil", TLSMode: "starttls"}} {
		if New(cfg).Configured() {
			t.Fatal("unsafe SMTP configuration accepted")
		}
	}
}
