package notify

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"sync"

	"github.com/jordan-wright/email"
	"go.uber.org/zap"
)

type SmtpSettings struct {
	Host     string
	Port     int
	Username string
	Password string
	Tls      bool
	auth     smtp.Auth
}

type EmailTmpl struct {
	Subject string
	Body    string
	// Notfound string
	Receivers    []string
	CcReceivers  []string
	BccReceivers []string
	tSub         *template.Template
	tBody        *template.Template
	// tNotfound *template.Template
}

type EmailNotifer struct {
	Logger *zap.Logger
	From   string
	//Receivers []string
	SMTP     SmtpSettings
	Template map[string]*EmailTmpl
	Once     sync.Once
}

func (en *EmailNotifer) PostInit() error {
	if en.Logger == nil {
		en.Logger = zap.L()
	}

	for _, item := range en.Template {
		item.tSub = template.Must(template.New("sub").Parse(item.Subject))
		item.tBody = template.Must(template.New("body").Parse(item.Body))
		// item.tNotfound = template.Must(template.New("body").Parse(item.Notfound))

		en.Logger.Debug("template's receivers", zap.Any("template", item))
	}

	en.Logger.Debug("template is ready")
	if en.SMTP.Username != "" {
		en.SMTP.auth = smtp.PlainAuth("", en.SMTP.Username, en.SMTP.Password, en.SMTP.Host)
		en.Logger.Info("send email with auth", zap.String("username", en.SMTP.Username))
	}
	return nil
}

func (en *EmailNotifer) Send(tmpl string, data map[string]interface{}, attachments ...string) error {
	en.Once.Do(func() {
		err := en.PostInit()
		if err != nil {
			panic(err)
		}
	})
	e := email.NewEmail()

	e.From = en.From
	//e.To = en.Receivers

	out := bytes.Buffer{}

	tmp, ok := en.Template[tmpl]
	if !ok {
		return fmt.Errorf("%s is missed from settings", tmpl)
	}
	en.Logger.Debug("template", zap.String("tmpl", tmpl))
	en.Logger.Debug("template is ", zap.Any("", tmp))
	e.To = tmp.Receivers
	e.Cc = tmp.CcReceivers
	e.Bcc = tmp.BccReceivers

	err := tmp.tSub.Execute(&out, data)
	if err != nil {
		en.Logger.Error("match email subject failed.", zap.Error(err))
		return err
	}

	e.Subject = out.String()

	out = bytes.Buffer{}

	// if attachments != nil {
	err = tmp.tBody.Execute(&out, data)
	if err != nil {
		en.Logger.Error("match email content failed.", zap.Error(err))
		return err
	}
	// } else {
	// 	err = tmp.tNotfound.Execute(&out, data)
	// 	if err != nil {
	// 		en.Logger.Error("match email content failed.", zap.Error(err))
	// 		return err
	// 	}
	// }
	// e.Text = out.Bytes()
	e.HTML = out.Bytes()

	for _, file := range attachments {
		_, err = e.AttachFile(file)
		if err != nil {
			en.Logger.Error("attach file failed", zap.String("file", file), zap.Error(err))
			return err
		}
		en.Logger.Info("attached file", zap.String("file", file))
	}
	fullAddress := fmt.Sprintf("%s:%d", en.SMTP.Host, en.SMTP.Port)

	en.Logger.Debug("start to send email", zap.String("smtp", fullAddress),
		zap.Strings("receivers", tmp.Receivers),
		zap.Bool("TLS", en.SMTP.Tls),
	)
	if en.SMTP.Tls {
		err = e.SendWithTLS(fullAddress, en.SMTP.auth, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         en.SMTP.Host,
		})
		// err = e.SendWithStartTLS(fullAddress, en.SMTP.auth, &tls.Config{})
	} else {
		err = e.Send(fullAddress, en.SMTP.auth)
	}

	if err != nil {
		en.Logger.Error("send email failed", zap.Error(err), zap.Strings("receivers", tmp.Receivers))
		return err
	}
	en.Logger.Info("send email done.", zap.Strings("receivers", tmp.Receivers))
	return nil
}
