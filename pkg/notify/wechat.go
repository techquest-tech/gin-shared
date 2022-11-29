package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/techquest-tech/gin-shared/pkg/untils/httpclient"
)

const (
	UrlGetToken = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	UrlSend     = "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s"
)

type WechatTmpl struct {
	Receivers map[string]string
	Content   string
	tContent  *template.Template
}

type TokenCache struct {
	Created time.Time
	Expired time.Duration
	Token   string
}

type WechatNotifer struct {
	CorpID     string
	Secret     string
	AgentID    string
	Template   map[string]WechatTmpl
	Logger     *zap.Logger
	tokenCache *TokenCache
}

type TokenResp struct {
	AccessToken string `json:"access_token"`
	Errcode     int    `json:"errcode"`
	Errmsg      string `json:"errmsg"`
	ExpiresIn   int    `json:"expires_in"`
}

type SendResp struct {
	Errcode        int    `json:"errcode"`
	Errmsg         string `json:"errmsg"`
	Invalidparty   string `json:"invalidparty"`
	Invalidtag     string `json:"invalidtag"`
	Invaliduser    string `json:"invaliduser"`
	Msgid          string `json:"msgid"`
	ResponseCode   string `json:"response_code"`
	Unlicenseduser string `json:"unlicenseduser"`
}

func (wn *WechatNotifer) PostInit() error {
	if wn.Logger == nil {
		wn.Logger = zap.L()
	}
	for _, item := range wn.Template {
		item.tContent = template.Must(template.New("wx").Parse(item.Content))
	}
	wn.Logger.Debug("wechat notify init done.")
	if wn.tokenCache == nil {
		wn.tokenCache = &TokenCache{}
	}
	return nil
}

func (wn *WechatNotifer) getToken() (string, error) {
	if wn.tokenCache.Token != "" {
		if time.Since(wn.tokenCache.Created) < wn.tokenCache.Expired {
			wn.Logger.Debug("get token from cache.")
			return wn.tokenCache.Token, nil
		}
		wn.Logger.Debug("token has expired. renew one")
	}
	p := url.Values{
		"corpid":     {wn.CorpID},
		"corpsecret": {wn.Secret},
	}
	req, _ := http.NewRequest("GET", p.Encode(), nil)

	result := TokenResp{}

	err := httpclient.RequestWithRetry(req, &result)
	if err != nil {
		wn.Logger.Info("request token from Wechat failed.", zap.Error(err))
		return "", err
	}
	if result.Errcode != 0 {
		wn.Logger.Error("get token failed.", zap.Any("result", result))
		return "", fmt.Errorf(result.Errmsg)
	}

	wn.tokenCache.Created = time.Now()
	wn.tokenCache.Token = result.AccessToken
	wn.tokenCache.Expired = time.Second * time.Duration(result.ExpiresIn)

	return result.AccessToken, nil
}

func (wn *WechatNotifer) Send(tmpl string, data map[string]interface{}) error {
	out := bytes.Buffer{}
	tmp, ok := wn.Template[tmpl]
	if !ok {
		return fmt.Errorf("template %s is not found", tmpl)
	}

	for key, value := range tmp.Receivers {
		data[key] = value
	}
	err := tmp.tContent.Execute(&out, data)
	if err != nil {
		wn.Logger.Error("Wechat template execute failed.", zap.Error(err))
		return err
	}

	token, err := wn.getToken()
	if err != nil {
		return err
	}

	sendurl := fmt.Sprintf(UrlSend, token)

	req, _ := http.NewRequest("POST", sendurl, bytes.NewBuffer(out.Bytes()))
	req.Header.Add("Content-Type", "application/json")

	resp := SendResp{}

	err = httpclient.RequestWithRetry(req, &resp)
	if err != nil {
		wn.Logger.Error("send wechat message failed.", zap.Error(err))
		return err
	}
	if resp.Errcode != 0 {
		wn.Logger.Error("send wechat message replied error", zap.Any("resp", resp))
		return fmt.Errorf(resp.Errmsg)
	}

	return err
}
