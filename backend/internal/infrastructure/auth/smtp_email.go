package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type smtpEmailSender struct {
	host, port, username, password, from string
}

func newSMTPEmailSenderFromEnv() EmailSender {
	sender := &smtpEmailSender{
		host: os.Getenv("SMTP_HOST"), port: os.Getenv("SMTP_PORT"),
		username: os.Getenv("SMTP_USERNAME"), password: os.Getenv("SMTP_PASSWORD"),
		from: os.Getenv("SMTP_FROM"),
	}
	if sender.host == "" || sender.port == "" || sender.username == "" || sender.password == "" || sender.from == "" {
		return nil
	}
	if _, err := strconv.Atoi(sender.port); err != nil {
		return nil
	}
	address, err := mail.ParseAddress(sender.from)
	if err != nil || address.Address != sender.from {
		return nil
	}
	return sender
}

func (s *smtpEmailSender) Send(ctx context.Context, address, code string) error {
	server := net.JoinHostPort(s.host, s.port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}
	if s.port == "465" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.port != "465" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
		return err
	}
	if err := client.Mail(s.from); err != nil {
		return err
	}
	if err := client.Rcpt(address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	subject := mime.QEncoding.Encode("utf-8", "慧投登录验证码")
	message := strings.Join([]string{
		"From: " + s.from, "To: " + address, "Subject: " + subject,
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8", "",
		"你的慧投验证码是 " + code + "，5 分钟内有效。若非本人操作，请忽略此邮件。", "",
	}, "\r\n")
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
