package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
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
	ReplyTo      []string
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

	mu        sync.Mutex
	namespace string
	initedAt  time.Time
}

// initTemplatesAndAuth 初始化模板渲染器与 SMTP 认证信息。
// 返回值：返回初始化过程中的错误信息。
func (en *EmailNotifer) initTemplatesAndAuth() error {
	if en.SMTP.Host == "" {
		viper.UnmarshalKey("smtp", &en.SMTP)
	}

	for _, item := range en.Template {
		item.tSub = template.Must(template.New("sub").Parse(item.Subject))
		item.tBody = template.Must(template.New("body").Parse(item.Body))
		en.Logger.Debug("template's receivers", zap.Any("template", item))
	}

	en.Logger.Debug("template is ready")
	if en.SMTP.Username != "" {
		en.SMTP.auth = smtp.PlainAuth("", en.SMTP.Username, en.SMTP.Password, en.SMTP.Host)
		en.Logger.Info("send email with auth", zap.String("username", en.SMTP.Username))
	}
	return nil
}

func (en *EmailNotifer) PostInit() error {
	if en.Logger == nil {
		en.Logger = zap.L()
	}

	en.mu.Lock()
	defer en.mu.Unlock()

	// 邮件通知配置优先从数据库加载；当数据库中缺失配置时，继续使用 viper/yaml 中已有配置。
	// 由于对外初始化接口不变，这里采用“根据模板名推断 namespace”并做最小侵入的读取。
	if err := en.tryLoadFromDBOrSkip(context.Background()); err != nil {
		return err
	}
	if err := en.initTemplatesAndAuth(); err != nil {
		return err
	}
	en.initedAt = time.Now()
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
	var smtpCfg SmtpSettings

	// 当启用 DB 配置时，为保证配置修改立即生效，每次发送前都会尝试从 DB 重新加载一次最新配置。
	// 为避免并发发送与配置热更新产生数据竞争，这里会在“构建邮件内容”阶段加锁。
	dsn := strings.TrimSpace(viper.GetString("database.connection"))
	if dsn != "" {
		en.mu.Lock()
		if err := en.tryLoadFromDBOrSkip(context.Background()); err != nil {
			en.mu.Unlock()
			return err
		}
		if err := en.initTemplatesAndAuth(); err != nil {
			en.mu.Unlock()
			return err
		}

		e.From = en.From

		out := bytes.Buffer{}
		tmp, ok := en.Template[tmpl]
		if !ok {
			en.mu.Unlock()
			return fmt.Errorf("%s is missed from settings", tmpl)
		}
		en.Logger.Debug("template", zap.String("tmpl", tmpl))
		en.Logger.Debug("template is ", zap.Any("", tmp))

		if len(to) > 0 {
			e.To = to
		}
		if len(tmp.Receivers) > 0 {
			e.To = append(e.To, tmp.Receivers...)
		}
		if len(cc) > 0 {
			e.Cc = cc
		} else {
			e.Cc = tmp.CcReceivers
		}
		if len(bcc) > 0 {
			e.Bcc = bcc
		} else {
			e.Bcc = tmp.BccReceivers
		}
		if len(tmp.ReplyTo) > 0 {
			e.ReplyTo = tmp.ReplyTo
		}

		err := tmp.tSub.Execute(&out, data)
		if err != nil {
			en.Logger.Error("match email subject failed.", zap.Error(err))
			en.mu.Unlock()
			return err
		}
		e.Subject = out.String()

		out = bytes.Buffer{}
		err = tmp.tBody.Execute(&out, data)
		if err != nil {
			en.Logger.Error("match email content failed.", zap.Error(err))
			en.mu.Unlock()
			return err
		}
		e.HTML = out.Bytes()

		smtpCfg = en.SMTP
		en.mu.Unlock()
	} else {
		e.From = en.From

		out := bytes.Buffer{}
		tmp, ok := en.Template[tmpl]
		if !ok {
			return fmt.Errorf("%s is missed from settings", tmpl)
		}
		en.Logger.Debug("template", zap.String("tmpl", tmpl))
		en.Logger.Debug("template is ", zap.Any("", tmp))

		if len(to) > 0 {
			e.To = to
		}
		if len(tmp.Receivers) > 0 {
			e.To = append(e.To, tmp.Receivers...)
		}
		if len(cc) > 0 {
			e.Cc = cc
		} else {
			e.Cc = tmp.CcReceivers
		}
		if len(bcc) > 0 {
			e.Bcc = bcc
		} else {
			e.Bcc = tmp.BccReceivers
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
		smtpCfg = en.SMTP
	}

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
	fullAddress := fmt.Sprintf("%s:%d", smtpCfg.Host, smtpCfg.Port)

	en.Logger.Debug("start to send email", zap.String("smtp", fullAddress),
		zap.Strings("receivers", e.To),
		zap.Bool("TLS", smtpCfg.Tls),
	)
	var err error
	if smtpCfg.Tls {
		err = e.SendWithTLS(fullAddress, smtpCfg.auth, &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         smtpCfg.Host,
		})
	} else {
		err = e.Send(fullAddress, smtpCfg.auth)
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
