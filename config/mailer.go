package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"

	mail "github.com/go-mail/mail/v2"
)

var (
	smtpHost = os.Getenv("SMTP_HOST")
	smtpPort = func() int {
		p, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
		if p == 0 {
			p = 587
		}
		return p
	}()
	smtpUser      = os.Getenv("SMTP_USERNAME")
	smtpPass      = os.Getenv("SMTP_PASSWORD")
	smtpFrom      = os.Getenv("SMTP_FROM") // e.g. "Fund System <no-reply@your.org>"
	skipTLSVerify = os.Getenv("SMTP_SKIP_TLS_VERIFY") == "1"
)

func SendMail(to []string, subject, html string) error {
	if len(to) == 0 {
		return nil
	}
	if smtpHost == "" || smtpFrom == "" {
		return fmt.Errorf("smtp not configured (SMTP_HOST/SMTP_FROM)")
	}

	m := mail.NewMessage()
	m.SetHeader("From", smtpFrom)
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", html)

	d := mail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify}

	return d.DialAndSend(m)
}
