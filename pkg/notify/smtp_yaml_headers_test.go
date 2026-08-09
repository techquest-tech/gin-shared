package notify_test

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/techquest-tech/gin-shared/pkg/notify"
)

const yamlConfig = `
smtp:
  host: smtp.example.com
  port: 465
  username: alerts@example.com
  password: secret
  tls: true

notify:
  from: alerts@example.com
  defaultTo:
    - default-to@example.com
  defaultCc:
    - default-cc@example.com
  defaultBcc:
    - audit@example.com
    - log@example.com
  defaultHeaders:
    X-Env: prod
    X-Mailer: gin-shared-test
    List-Unsubscribe: "<mailto:unsub@example.com?subject=out>"
    X-Priority: "3"
    Importance: normal
  template:
    p0-incident:
      subject: "[P0] {{.IncidentID}} 生产事故告警"
      body: "<h1>{{.Title}}</h1>"
      receivers:
        - oncall@example.com
        - incident@example.com
      ccReceivers:
        - techlead@example.com
      bccReceivers:
        - dba@example.com
      replyTo:
        - reply-incident@example.com
      priority: 1
      headers:
        X-Priority: "1"
        Importance: high
        X-Alert-Level: P0
        X-Incident-ID: INC-001
        "X-Special-Key": "value with : colon and space"
        X-Numeric-Value: "12345"
        "List-Unsubscribe": "<https://example.com/unsub>"
        Auto-Submitted: auto-generated
        X-Nested.Value: dot.key.allowed

    daily-report:
      subject: "日报 {{.Date}}"
      body: "摘要..."
      receivers:
        - team@example.com
      # 不设置 priority 也不设置 headers -> 应不包含优先级头，应继承 defaultHeaders 中的 X-Priority/Importance

    promo:
      subject: "营销活动"
      body: "促销"
      receivers:
        - all@example.com
      headers:
        X-Priority: "5"
        Importance: low
        Precedence: bulk
        X-Campaign: SUMMER-2026
`

func loadConfigFromYAML(t *testing.T, yaml string, callPostInit bool) *notify.EmailNotifer {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewBufferString(yaml))
	if err != nil {
		t.Fatalf("viper ReadConfig yaml failed: %v", err)
	}

	var en notify.EmailNotifer
	en.From = v.GetString("notify.from")
	en.DefaultTo = v.GetStringSlice("notify.defaultTo")
	en.DefaultCc = v.GetStringSlice("notify.defaultCc")
	en.DefaultBcc = v.GetStringSlice("notify.defaultBcc")

	err = v.UnmarshalKey("notify.defaultHeaders", &en.DefaultHeaders)
	if err != nil {
		t.Fatalf("UnmarshalKey notify.defaultHeaders failed: %v", err)
	}
	err = v.UnmarshalKey("smtp", &en.SMTP)
	if err != nil {
		t.Fatalf("UnmarshalKey smtp failed: %v", err)
	}
	err = v.UnmarshalKey("notify.template", &en.Template)
	if err != nil {
		t.Fatalf("UnmarshalKey notify.template failed: %v", err)
	}
	if callPostInit {
		err := en.PostInit()
		if err != nil {
			t.Fatalf("PostInit failed: %v", err)
		}
	}
	return &en
}

func TestViperYAMLLoadRawHeadersWithoutPostInit(t *testing.T) {
	en := loadConfigFromYAML(t, yamlConfig, false)

	assert.NotNil(t, en.DefaultHeaders, "DefaultHeaders 不能为 nil")
	raw := map[string]string{
		"x-env":           "prod",
		"x-mailer":        "gin-shared-test",
		"list-unsubscribe": "<mailto:unsub@example.com?subject=out>",
		"x-priority":      "3",
		"importance":      "normal",
	}
	for k, v := range raw {
		assert.Equalf(t, v, en.DefaultHeaders[k], "DefaultHeaders[%q] (raw lowercase key) 不匹配", k)
	}
	t.Logf("raw DefaultHeaders (viper 小写化后): %+v", en.DefaultHeaders)

	tmpl := en.Template["p0-incident"]
	assert.NotNil(t, tmpl.Headers, "tmpl.Headers 不能为 nil")
	for k, v := range map[string]string{
		"x-priority":    "1",
		"importance":    "high",
		"x-alert-level": "P0",
	} {
		assert.Equalf(t, v, tmpl.Headers[k], "tmpl.Headers[%q] (raw lowercase key) 不匹配", k)
	}
	t.Logf("raw tmpl.Headers (viper 小写化后): %+v", tmpl.Headers)
}

func TestViperYAMLLoadDefaultHeaders(t *testing.T) {
	en := loadConfigFromYAML(t, yamlConfig, true)

	assert.Equal(t, "alerts@example.com", en.From)
	assert.Equal(t, []string{"default-to@example.com"}, en.DefaultTo)
	assert.Equal(t, []string{"default-cc@example.com"}, en.DefaultCc)
	assert.Equal(t, []string{"audit@example.com", "log@example.com"}, en.DefaultBcc)

	assert.NotNil(t, en.DefaultHeaders, "DefaultHeaders 不能为 nil")
	assert.Equal(t, "prod", en.DefaultHeaders["X-Env"])
	assert.Equal(t, "gin-shared-test", en.DefaultHeaders["X-Mailer"])
	assert.Equal(t, "<mailto:unsub@example.com?subject=out>", en.DefaultHeaders["List-Unsubscribe"])
	assert.Equal(t, "3", en.DefaultHeaders["X-Priority"])
	assert.Equal(t, "normal", en.DefaultHeaders["Importance"])
	t.Logf("DefaultHeaders (PostInit 规范化后): %+v", en.DefaultHeaders)
}

func TestViperYAMLLoadTemplateP0Headers(t *testing.T) {
	en := loadConfigFromYAML(t, yamlConfig, true)

	tmpl, ok := en.Template["p0-incident"]
	assert.True(t, ok, "p0-incident 模板必须存在")
	assert.NotNil(t, tmpl)

	assert.Equal(t, []string{"oncall@example.com", "incident@example.com"}, tmpl.Receivers)
	assert.Equal(t, []string{"techlead@example.com"}, tmpl.CcReceivers)
	assert.Equal(t, []string{"dba@example.com"}, tmpl.BccReceivers)
	assert.Equal(t, []string{"reply-incident@example.com"}, tmpl.ReplyTo)
	assert.Equal(t, notify.EmailPriority(1), tmpl.Priority, "priority: 1 应解析为 EmailPriorityHighest")

	assert.NotNil(t, tmpl.Headers, "tmpl.Headers 不能为 nil")

	expected := map[string]string{
		"X-Priority":       "1",
		"Importance":    "high",
		"X-Alert-Level": "P0",
		"X-Incident-Id": "INC-001",
		"X-Special-Key": "value with : colon and space",
		"X-Numeric-Value": "12345",
		"List-Unsubscribe": "<https://example.com/unsub>",
		"Auto-Submitted": "auto-generated",
		"X-Nested.value": "dot.key.allowed",
	}
	for k, v := range expected {
		actual, has := tmpl.Headers[k]
		assert.Truef(t, has, "tmpl.Headers 缺少 Key %q，实际 Headers = %+v", k, tmpl.Headers)
		assert.Equalf(t, v, actual, "tmpl.Headers[%q] 不匹配", k)
	}
	t.Logf("p0-incident Headers (PostInit 规范化后): %+v", tmpl.Headers)

	h := tmpl.Priority.Headers()
	assert.Equal(t, "1", h.Get("X-Priority"))
	assert.Equal(t, "high", h.Get("Importance"))
}

func TestViperYAMLLoadTemplateDailyReportNoHeaders(t *testing.T) {
	en := loadConfigFromYAML(t, yamlConfig, true)

	tmpl, ok := en.Template["daily-report"]
	assert.True(t, ok, "daily-report 模板必须存在")
	assert.NotNil(t, tmpl)

	assert.Equal(t, notify.EmailPriority(0), tmpl.Priority, "未设置 priority 字段应为 0(零值)")
	assert.False(t, tmpl.Priority.IsValid(), "零值 Priority 不应被视为有效")

	// yaml 中未写 headers 字段 → viper Unmarshal 到 map[string]string 时为 nil(或空map，取决于实现)，但不报错
	if tmpl.Headers == nil {
		t.Log("daily-report tmpl.Headers 为 nil（未在 yaml 中声明）")
	} else {
		t.Logf("daily-report tmpl.Headers 为 空 map: %+v", tmpl.Headers)
		assert.Empty(t, tmpl.Headers, "yaml 中未定义 headers 应得到空或 nil")
	}
}

func TestViperYAMLLoadTemplatePromoHeaders(t *testing.T) {
	en := loadConfigFromYAML(t, yamlConfig, true)

	tmpl, ok := en.Template["promo"]
	assert.True(t, ok)
	assert.NotNil(t, tmpl)

	assert.Equal(t, notify.EmailPriority(0), tmpl.Priority, "promo 模板 priority 未设置 => 0")
	assert.NotNil(t, tmpl.Headers)
	assert.Equal(t, "5", tmpl.Headers["X-Priority"])
	assert.Equal(t, "low", tmpl.Headers["Importance"])
	assert.Equal(t, "bulk", tmpl.Headers["Precedence"])
	assert.Equal(t, "SUMMER-2026", tmpl.Headers["X-Campaign"])
	t.Logf("promo Headers (PostInit 规范化后): %+v", tmpl.Headers)
}
