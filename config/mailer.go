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
	smtpUser      = os.Getenv("SMTP_USER")
	smtpPass      = os.Getenv("SMTP_PASS")
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

	// ใช้ STARTTLS บนพอร์ต 587 แบบบังคับ (เหมาะกับ Gmail/Office365)
	d.StartTLSPolicy = mail.MandatoryStartTLS

	// แก้ TLS: ต้องมี ServerName หรือ InsecureSkipVerify
	d.TLSConfig = &tls.Config{
		ServerName:         smtpHost,      // สำคัญ! ให้ตรงกับ hostname เช่น "smtp.gmail.com"
		InsecureSkipVerify: skipTLSVerify, // dev เท่านั้น: ตั้ง .env เป็น 1 หากต้องข้ามการตรวจ cert
	}

	return d.DialAndSend(m)
}
