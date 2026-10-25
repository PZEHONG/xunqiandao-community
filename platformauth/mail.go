package platformauth

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Mailer interface {
	Ready() bool
	SendCode(context.Context, string, string, string) error
}
type SMTPMailer struct{}
type smtpConfig struct{ host, port, username, password, from string }

// Configuration is read on each send, so local .env edits need no restart.
func authConfig() func(string) string {
	values := map[string]string{}
	path := os.Getenv("AUTH_CONFIG_FILE")
	if path == "" {
		path = ".env"
	}
	if f, e := os.Open(path); e == nil {
		defer f.Close()
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			line := strings.TrimSpace(scan.Text())
			if strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if ok {
				values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
			}
		}
	}
	get := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(value)
		}
		return values[key]
	}
	return get
}
func loadSMTP() smtpConfig {
	get := authConfig()
	c := smtpConfig{get("SMTP_HOST"), get("SMTP_PORT"), get("SMTP_USERNAME"), get("SMTP_PASSWORD"), get("SMTP_FROM")}
	if c.username == "" {
		c.username = get("SMTP_USER")
	}
	if c.password == "" {
		c.password = get("SMTP_PASS")
	}
	if c.port == "" {
		c.port = "465"
	}
	if c.from == "" {
		c.from = c.username
	}
	return c
}
func validSMTP(c smtpConfig) bool {
	address, e := mail.ParseAddress(c.from)
	port, pe := strconv.Atoi(c.port)
	return c.host != "" && !strings.ContainsAny(c.host, "\r\n/") && pe == nil && port > 0 && port < 65536 && c.username != "" && c.password != "" && e == nil && address.Address == c.from
}
func (SMTPMailer) Ready() bool { return validSMTP(loadSMTP()) }
func (SMTPMailer) SendCode(ctx context.Context, email, code, purpose string) error {
	c := loadSMTP()
	if !validSMTP(c) {
		return fmt.Errorf("SMTP unavailable")
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var e error
	address := net.JoinHostPort(c.host, c.port)
	tlsConfig := &tls.Config{ServerName: c.host, MinVersion: tls.VersionTLS12}
	if c.port == "465" {
		conn, e = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, e = dialer.DialContext(ctx, "tcp", address)
	}
	if e != nil {
		return e
	}
	defer conn.Close()
	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	client, e := smtp.NewClient(conn, c.host)
	if e != nil {
		return e
	}
	defer client.Close()
	if c.port != "465" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP requires TLS")
		}
		if e = client.StartTLS(tlsConfig); e != nil {
			return e
		}
	}
	if e = client.Auth(smtp.PlainAuth("", c.username, c.password, c.host)); e != nil {
		return e
	}
	if e = client.Mail(c.from); e != nil {
		return e
	}
	if e = client.Rcpt(email); e != nil {
		return e
	}
	writer, e := client.Data()
	if e != nil {
		return e
	}
	action := "注册账号"
	if purpose == "reset" {
		action = "重置密码"
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: =?UTF-8?B?6K6v562+5Yiw6aqM6K+B56CB?=\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n你正在%s，讯签到验证码为：%s\r\n\r\n验证码 10 分钟内有效，请勿透露给他人。如果不是你本人操作，请忽略此邮件。\r\n", c.from, email, action, code)
	if _, e = writer.Write([]byte(message)); e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	return client.Quit()
}
