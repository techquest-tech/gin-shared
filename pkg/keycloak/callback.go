package keycloak

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type KeycloakFullSettings struct {
	URL          string
	Realm        string
	ClientID     string
	ClientSecret string
	Endpoint     string
}

var keycloakSettings *KeycloakFullSettings

var oauthConfig *oauth2.Config

// var buildconfig ginkeycloak.KeycloakConfig

func init() {
	core.ProvideStartup(func(logger *zap.Logger) core.Startup {
		keycloakSettings = &KeycloakFullSettings{}
		sub := viper.Sub("auth.config")
		if sub != nil {
			sub.Unmarshal(keycloakSettings)

			keycloakEndpoint := keycloakSettings.URL + "/realms/" + keycloakSettings.Realm
			keycloakSettings.Endpoint = keycloakEndpoint
			logger.Info("keycloak settings loaded", zap.String("endpoint", keycloakEndpoint))

			oauthConfig = &oauth2.Config{
				ClientID:     keycloakSettings.ClientID,
				ClientSecret: keycloakSettings.ClientSecret,
				Endpoint: oauth2.Endpoint{
					AuthURL:  keycloakEndpoint + "/protocol/openid-connect/auth",
					TokenURL: keycloakEndpoint + "/protocol/openid-connect/token",
				},
				Scopes: []string{"openid", "profile", "email", "roles"},
			}
			// buildconfig = ginkeycloak.KeycloakConfig{
			// 	Realm: keycloakSettings.Realm,
			// 	Url:   keycloakSettings.URL,
			// }
		}

		return nil
	})
}

func RedirectKeyCloakLogin(c *gin.Context, redirectURI string) {
	if oauthConfig == nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	schema := "http"
	if c.Request.TLS != nil {
		schema = "https"
	}

	redirectTo := fmt.Sprintf("%s://%s%s", schema, c.Request.Host, redirectURI)
	oauthConfig.RedirectURL = redirectTo
	url := oauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusSeeOther, url)
}

func KeycloakCallback() gin.HandlerFunc {
	return func(c *gin.Context) {
		if oauthConfig == nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		code := c.Query("code")
		if code == "" {
			c.String(http.StatusBadRequest, "Code not found")
			return
		}
		token, err := oauthConfig.Exchange(c, code)
		if err != nil {
			// c.String(http.StatusInternalServerError, "Failed to exchange token: %v", err)
			zap.L().Error("exchange token failed", zap.Error(err))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		zap.L().Info("exchange token success")
		c.Set("oauth2", token)

		// tk2, err := ginkeycloak.GetTokenContainer(token, buildconfig)
		// if err != nil {
		// 	zap.L().Error("get token container failed", zap.Error(err))
		// 	c.AbortWithStatus(http.StatusInternalServerError)
		// }

		// c.Set("_token", tk2)
		// client := oauthConfig.Client(c, token)
		// resp, err := client.Get(fmt.Sprintf("%s/protocol/openid-connect/userinfo", keycloakSettings.Endpoint))

		// if err != nil {
		// 	c.String(http.StatusInternalServerError, "Failed to get user info: %v", err)
		// 	return
		// }
		// defer resp.Body.Close()

		// var userInfo map[string]interface{}
		// if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		// 	c.String(http.StatusInternalServerError, "Failed to decode user info: %v", err)
		// 	return
		// }
		// c.Set("currentUser", userInfo)
		c.Next()
	}
}
