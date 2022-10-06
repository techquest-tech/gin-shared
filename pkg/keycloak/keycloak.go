package keycloak

import (
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
	return func(ctx *gin.Context) {
		if logCurrentUser {
			tk, ok := ctx.Get("token")
			if ok {
				// token:=tk.(ginkeycloak.KeyCloakToken)
				zap.L().Debug("keycloak token", zap.Any("token", tk))
			} else {
				zap.L().Warn("no token provided.")
			}
		}
		ginkeycloak.Auth(ginkeycloak.AuthCheck(), buildconfig)
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

	logger.Info("load keycloak config", zap.Any("config", config))

	return config
}
