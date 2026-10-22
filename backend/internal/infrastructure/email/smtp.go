package email

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
	"strconv"
	"strings"
	"time"
)

type Config struct{ Host, Port, Username, Password, From, TLSMode string }
type Client struct {
	cfg   Config
	roots *x509.CertPool // nil uses system trust; tests install their local CA.
}

func New(c Config) *Client { return &Client{cfg: c} }
func (c *Client) Configured() bool {
	port, err := strconv.Atoi(c.cfg.Port)
	from, fe := mail.ParseAddress(c.cfg.From)
	return c.cfg.Host != "" && err == nil && port > 0 && port <= 65535 && fe == nil && from.Address != "" && (c.cfg.TLSMode == "tls" || c.cfg.TLSMode == "starttls") && ((c.cfg.Username == "" && c.cfg.Password == "") || (c.cfg.Username != "" && c.cfg.Password != ""))
}

// SMTP is always encrypted and certificate-verified. Credentials never enter URLs.
func (c *Client) Send(ctx context.Context, to, subject, body, messageID string) error {
	if !c.Configured() {
		return errors.New("邮件服务未配置")
	}
	from, _ := mail.ParseAddress(c.cfg.From)
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return errors.New("收件邮箱无效")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsConfig := &tls.Config{ServerName: c.cfg.Host, MinVersion: tls.VersionTLS12, RootCAs: c.roots}
	var conn net.Conn
	if c.cfg.TLSMode == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", net.JoinHostPort(c.cfg.Host, c.cfg.Port))
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(c.cfg.Host, c.cfg.Port))
	}
	if err != nil {
		return errors.New("邮件服务器连接失败")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		return errors.New("邮件服务握手失败")
	}
	defer client.Close()
	if c.cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("邮件服务器不支持加密连接")
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return errors.New("邮件加密连接失败")
		}
	}
	if c.cfg.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, c.cfg.Host)); err != nil {
			return errors.New("邮件服务认证失败")
		}
	}
	if err = client.Mail(from.Address); err != nil {
		return errors.New("发件地址被拒绝")
	}
	if err = client.Rcpt(recipient.Address); err != nil {
		return errors.New("收件地址被拒绝")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("邮件提交失败")
	}
	if _, err = writer.Write([]byte(message(from, recipient, subject, body, messageID, time.Now()))); err != nil {
		return errors.New("邮件提交失败")
	}
	if err = writer.Close(); err != nil {
		return errors.New("邮件发送结果未确认")
	}
	// DATA's positive final reply confirms acceptance. A failed QUIT must not
	// turn a successfully accepted message into a retry and duplicate delivery.
	_ = client.Quit()
	return nil
}
func message(from, to *mail.Address, subject, body, id string, now time.Time) string {
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	domain := strings.Split(from.Address, "@")
	messageID := fmt.Sprintf("%s@%s", id, domain[len(domain)-1])
	messageID = strings.NewReplacer("\r", "", "\n", "", "<", "", ">", "").Replace(messageID)
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var lines []string
	for len(encoded) > 76 {
		lines = append(lines, encoded[:76])
		encoded = encoded[76:]
	}
	lines = append(lines, encoded)
	return "From: " + from.String() + "\r\nTo: " + to.String() + "\r\nSubject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\nDate: " + now.Format(time.RFC1123Z) + "\r\nMessage-ID: <" + messageID + ">\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + strings.Join(lines, "\r\n") + "\r\n"
}
