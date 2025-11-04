package controllers

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const emailLogoRelativePath = "templates/email_assets/fund_cpkku_logo.png"

var (
	emailLogoOnce sync.Once
	emailLogoHTML string
)

func getEmailLogoHTML() string {
	emailLogoOnce.Do(func() {
		path := filepath.FromSlash(emailLogoRelativePath)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("email header logo not found at %s: %v", path, err)
			emailLogoHTML = ""
			return
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		emailLogoHTML = fmt.Sprintf(
			`<img src="data:image/png;base64,%s" alt="ระบบบริหารจัดการทุนวิจัย" `+
				`style="display:block;width:72px;height:auto;margin:0 auto 18px auto;" />`,
			encoded,
		)
	})
	return emailLogoHTML
}
