package notify_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/techquest-tech/gin-shared/pkg/notify"
	"go.uber.org/zap"
)

func TestSSEmail(t *testing.T) {
	n := &notify.EmailNotifer{
		Logger: zap.L(),
		From:   "tech_support@summation.solutions",
		// SMTP: notify.SmtpSettings{
		// 	Host: "10.253.1.76",
		// 	Port: 25,
		// },
		SMTP: notify.SmtpSettings{
			Host:     "web1020.dataplugs.com",
			Port:     465,
			Username: "tech_support@summation.solutions",
			Password: os.Getenv("SMTP_PWD"),
			Tls:      true,
		},
		Template: map[string]*notify.EmailTmpl{
			"hello": {
				Subject:   "email test 2",
				Body:      "it's testing email",
				Receivers: []string{"Benedict@summationsolutions.com", "panarm@esquel.com", "armenpn@gmail.com", "107357752@qq.com"},
			},
		},
	}
	// n.PostInit()
	err := n.Send("hello", map[string]interface{}{})
	assert.Nil(t, err)
}
