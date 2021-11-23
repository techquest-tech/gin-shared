package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/tbaehler/gin-keycloak/pkg/ginkeycloak"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func init() {
	ginshared.GetContainer().Provide(NewKeycloakConfig)
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
