package notify_test

import (
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
			Password: "1q@W3e$R",
			Tls:      true,
		},
		Template: map[string]*notify.EmailTmpl{
			"hello": {
				Subject:   "email test",
				Body:      "it's testing email",
				Receivers: []string{"armen@summationsolutions.com", "armenpn@gmail.com"},
			},
		},
	}
	// n.PostInit()
	err := n.Send("hello", map[string]interface{}{})
	assert.Nil(t, err)
}
