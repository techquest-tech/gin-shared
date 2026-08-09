package notify

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type EmailPriority int

const (
	EmailPriorityHighest EmailPriority = 1
	EmailPriorityHigh    EmailPriority = 2
	EmailPriorityNormal  EmailPriority = 3
	EmailPriorityLow     EmailPriority = 4
	EmailPriorityLowest  EmailPriority = 5
)

func (p EmailPriority) IsValid() bool {
	return p >= EmailPriorityHighest && p <= EmailPriorityLowest
}

func (p EmailPriority) Headers() textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	if !p.IsValid() {
		return h
	}
	h.Set("X-Priority", fmt.Sprintf("%d", p))
	switch p {
	case EmailPriorityHighest, EmailPriorityHigh:
		h.Set("Importance", "high")
	case EmailPriorityLow, EmailPriorityLowest:
		h.Set("Importance", "low")
	default:
		h.Set("Importance", "normal")
	}
	return h
}

func normalizeHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return h
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[textproto.CanonicalMIMEHeaderKey(k)] = v
	}
	return out
}

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
	ReplyTo      []string
	Priority     EmailPriority
	Headers      map[string]string
	tSub         *template.Template
	tBody        *template.Template
	// tNotfound *template.Template
}

type EmailNotifer struct {
	Logger         *zap.Logger
	From           string
	DefaultTo      []string
	DefaultCc      []string
	DefaultBcc     []string
	DefaultHeaders map[string]string
	SMTP           SmtpSettings
	Template       map[string]*EmailTmpl
	Once           sync.Once
}

func (en *EmailNotifer) PostInit() error {
	if en.Logger == nil {
		en.Logger = zap.L()
	}

	if en.SMTP.Host == "" {
		viper.UnmarshalKey("smtp", &en.SMTP)
	}

	for _, item := range en.Template {
		item.tSub = template.Must(template.New("sub").Parse(item.Subject))
		item.tBody = template.Must(template.New("body").Parse(item.Body))
		item.Headers = normalizeHeaders(item.Headers)

		en.Logger.Debug("template's receivers", zap.Any("template", item))
	}

	en.DefaultHeaders = normalizeHeaders(en.DefaultHeaders)

	en.Logger.Debug("template is ready")
	if en.SMTP.Username != "" {
		en.SMTP.auth = smtp.PlainAuth("", en.SMTP.Username, en.SMTP.Password, en.SMTP.Host)
		en.Logger.Info("send email with auth", zap.String("username", en.SMTP.Username))
	}
	return nil
}

func (en *EmailNotifer) Send(tmpl string, data map[string]interface{}, attachments ...string) error {
	return en.SendTo(tmpl, nil, nil, nil, data, attachments...)
}

func (en *EmailNotifer) SendTo(tmpl string, to []string, cc []string, bcc []string, data map[string]interface{}, attachments ...string) error {
	en.Once.Do(func() {
		err := en.PostInit()
		if err != nil {
			panic(err)
		}
	})
	e := email.NewEmail()

	e.From = en.From

	out := bytes.Buffer{}

	tmp, ok := en.Template[tmpl]
	if !ok {
		return fmt.Errorf("%s is missed from settings", tmpl)
	}
	en.Logger.Debug("template", zap.String("tmpl", tmpl))
	en.Logger.Debug("template is ", zap.Any("", tmp))

	if len(to) > 0 {
		e.To = append(e.To, to...)
	}
	if len(tmp.Receivers) > 0 {
		e.To = append(e.To, tmp.Receivers...)
	}
	if len(en.DefaultTo) > 0 {
		e.To = append(e.To, en.DefaultTo...)
	}
	if len(cc) > 0 {
		e.Cc = append(e.Cc, cc...)
	}
	if len(tmp.CcReceivers) > 0 {
		e.Cc = append(e.Cc, tmp.CcReceivers...)
	}
	if len(en.DefaultCc) > 0 {
		e.Cc = append(e.Cc, en.DefaultCc...)
	}
	if len(bcc) > 0 {
		e.Bcc = append(e.Bcc, bcc...)
	}
	if len(tmp.BccReceivers) > 0 {
		e.Bcc = append(e.Bcc, tmp.BccReceivers...)
	}
	if len(en.DefaultBcc) > 0 {
		e.Bcc = append(e.Bcc, en.DefaultBcc...)
	}
	if len(tmp.ReplyTo) > 0 {
		e.ReplyTo = tmp.ReplyTo
	}

	err := tmp.tSub.Execute(&out, data)
	if err != nil {
		en.Logger.Error("match email subject failed.", zap.Error(err))
		return err
	}
	e.Subject = out.String()

	out = bytes.Buffer{}
	err = tmp.tBody.Execute(&out, data)
	if err != nil {
		en.Logger.Error("match email content failed.", zap.Error(err))
		return err
	}
	e.HTML = out.Bytes()

	for _, file := range attachments {
		if _, statErr := os.Stat(file); statErr != nil {
			en.Logger.Error("attachment not found", zap.String("file", file), zap.Error(statErr))
			return statErr
		}
		att, attErr := e.AttachFile(file)
		if attErr != nil {
			en.Logger.Error("attach file failed", zap.String("file", file), zap.Error(attErr))
			return attErr
		}

		if isImage(file) {
			fileName := filepath.Base(file)
			if att.Header != nil {
				att.Header.Set("Content-ID", fmt.Sprintf("<%s>", fileName))
				att.Header.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", fileName))
			}
			en.Logger.Info("attached inline image", zap.String("file", file), zap.String("cid", fileName))
		} else {
			en.Logger.Info("attached file", zap.String("file", file))
		}
	}
	if tmp.Priority.IsValid() {
		for k, vs := range tmp.Priority.Headers() {
			for _, v := range vs {
				e.Headers.Add(k, v)
			}
		}
		en.Logger.Debug("email priority set", zap.Int("priority", int(tmp.Priority)))
	}
	for k, v := range en.DefaultHeaders {
		e.Headers.Set(textproto.CanonicalMIMEHeaderKey(k), v)
	}
	for k, v := range tmp.Headers {
		e.Headers.Set(textproto.CanonicalMIMEHeaderKey(k), v)
	}
	fullAddress := fmt.Sprintf("%s:%d", en.SMTP.Host, en.SMTP.Port)

	en.Logger.Debug("start to send email", zap.String("smtp", fullAddress),
		zap.Strings("receivers", e.To),
		zap.Bool("TLS", en.SMTP.Tls),
	)
	if en.SMTP.Tls {
		err = e.SendWithTLS(fullAddress, en.SMTP.auth, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         en.SMTP.Host,
		})
	} else {
		err = e.Send(fullAddress, en.SMTP.auth)
	}

	if err != nil {
		en.Logger.Error("send email failed", zap.Error(err), zap.Strings("receivers", e.To))
		return err
	}
	en.Logger.Info("send email done.", zap.Strings("receivers", e.To))
	return nil
}

func isImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return true
	}
	return false
}
