package keycloak

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/tbaehler/gin-keycloak/pkg/ginkeycloak"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func MustLogin() gin.HandlerFunc {
	keycloakconfig := viper.Sub("keycloak")
	buildconfig := ginkeycloak.KeycloakConfig{}
	if keycloakconfig != nil {
		keycloakconfig.Unmarshal(&buildconfig)
	}
	logCurrentUser := viper.GetBool("keycloak.debug")
	keycloakFunc := ginkeycloak.Auth(ginkeycloak.AuthCheck(), buildconfig)

	return func(ctx *gin.Context) {
		keycloakFunc(ctx)
		if logCurrentUser {
			tk, ok := ctx.Get("token")
			if ok {
				token := tk.(ginkeycloak.KeyCloakToken)
				auth := time.Unix(token.Iat, 0)
				exp := time.Unix(token.Exp, 0)

				zap.L().Debug("keycloak token",
					zap.String("user", token.PreferredUsername),
					zap.Time("issue at", auth),
					zap.Time("expire at", exp),
					zap.String("session", token.SessionState),
				)
			} else {
				zap.L().Warn("no token provided.")
			}
		}
	}
}

func init() {
	core.GetContainer().Provide(NewKeycloakConfig)
}

type KeycloakConfig struct {
	DefaultRoles []string
	BuildConfig  ginkeycloak.BuilderConfig
}

func (kc *KeycloakConfig) Auth(roles ...string) gin.HandlerFunc {
	if len(roles) == 0 {
		roles = kc.DefaultRoles
	}
	x := ginkeycloak.NewAccessBuilder(kc.BuildConfig)
	for _, role := range roles {
		x = x.RestrictButForRealm(role)
	}
	return x.Build()
}

func NewKeycloakConfig(logger *zap.Logger) *KeycloakConfig {
	config := &KeycloakConfig{}

	settings := viper.Sub("keycloak")

	if settings == nil {
		logger.Error("keycloak settings are missing.")
		return nil
	}

	buildconfig := ginkeycloak.BuilderConfig{}
	settings.Unmarshal(&buildconfig)
	config.BuildConfig = buildconfig
	config.DefaultRoles = settings.GetStringSlice("roles")

	logger.Info("load keycloak config", zap.Any("config", config.BuildConfig.Url))

	return config
}
