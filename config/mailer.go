package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	mail "github.com/go-mail/mail/v2"
)

var (
	mailerOnce sync.Once
	mailerMu   sync.RWMutex

	smtpHost      string
	smtpPort      int
	smtpUser      string
	smtpPass      string
	smtpFrom      string
	skipTLSVerify bool
)

func loadMailerConfig() {
	mailerMu.Lock()
	defer mailerMu.Unlock()

	smtpHost = strings.TrimSpace(os.Getenv("SMTP_HOST"))

	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if port == 0 {
		port = 587
	}
	smtpPort = port

	smtpUser = os.Getenv("SMTP_USER")
	smtpPass = os.Getenv("SMTP_PASS")
	smtpFrom = strings.TrimSpace(os.Getenv("SMTP_FROM"))
	skipTLSVerify = os.Getenv("SMTP_SKIP_TLS_VERIFY") == "1"
}

func ensureMailerConfig() {
	mailerOnce.Do(loadMailerConfig)
}

// ReloadMailerConfig forces the mailer configuration to be reloaded from the environment.
// It should be called after environment variables are changed at runtime (e.g. after loading .env).
func ReloadMailerConfig() {
	mailerMu.Lock()
	mailerOnce = sync.Once{}
	mailerMu.Unlock()
	ensureMailerConfig()
}

func SendMail(to []string, subject, html string) error {
	if len(to) == 0 {
		return nil
	}
	ensureMailerConfig()

	mailerMu.RLock()
	host := smtpHost
	port := smtpPort
	user := smtpUser
	pass := smtpPass
	from := smtpFrom
	skipVerify := skipTLSVerify
	mailerMu.RUnlock()

	if host == "" || from == "" {
		return fmt.Errorf("smtp not configured (SMTP_HOST/SMTP_FROM)")
	}

	m := mail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", html)

	d := mail.NewDialer(host, port, user, pass)

	// ใช้ STARTTLS บนพอร์ต 587 แบบบังคับ (เหมาะกับ Gmail/Office365)
	d.StartTLSPolicy = mail.MandatoryStartTLS

	// แก้ TLS: ต้องมี ServerName หรือ InsecureSkipVerify
	d.TLSConfig = &tls.Config{
		ServerName:         host,       // สำคัญ! ให้ตรงกับ hostname เช่น "smtp.gmail.com"
		InsecureSkipVerify: skipVerify, // dev เท่านั้น: ตั้ง .env เป็น 1 หากต้องข้ามการตรวจ cert
	}

	return d.DialAndSend(m)
}
