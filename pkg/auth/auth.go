package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/dig"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AuthServiceParam struct {
	dig.In
	DB     *gorm.DB `optional:"true"`
	Logger *zap.Logger
}

const (
	KeyUser = "currentUser"
)

func init() {
	ginshared.GetContainer().Provide(func(ap AuthServiceParam) *AuthService {
		authService := &AuthService{
			Db:     ap.DB,
			logger: ap.Logger,
		}
		authSetting := viper.Sub("auth")
		if authSetting != nil {
			if authService != nil {
				authSetting.Unmarshal(authService)
			}
		}

		if viper.GetBool(ginshared.KeyInitDB) {
			ap.DB.AutoMigrate(&AuthKey{})
		}

		return authService
	})
	ginshared.GetContainer().Provide(NewRouterRequiredAuth)
}

type AuthKey struct {
	gorm.Model
	ApiKey string `gorm:"size:64"`
	Owner  string `gorm:"size:64"`
	Remark string `gorm:"size:64"`
}

type AuthService struct {
	Db     *gorm.DB
	logger *zap.Logger
	// SQL    string
	Keys []string
}

// const CheckSql = "SELECT id,remark from appusers a where a.IsDeleted = 0 and a.AppKey = ?"

func (a *AuthService) checkKey(key string) (uint, bool) {

	for _, k := range a.Keys {
		if k == key {
			a.logger.Info("use build-in key")
			return 0, true
		}
	}

	if a.Db == nil {
		a.logger.Warn("DB is not enabled for Auth service.")
		return 0, false
	}

	authkey := AuthKey{}
	err := a.Db.First(&authkey, "api_key = ?", key).Error

	if err != nil {
		a.logger.Error("sql query error", zap.Any("error", err))
		return 0, false
	}

	return authkey.ID, true
}

func (a *AuthService) Auth(c *gin.Context) {
	key := ""
	switch c.Request.Method {
	case "GET":
		key = c.Query("apiKey")
	default:
		key = c.PostForm("apiKey")
	}
	if key == "" {
		key = c.GetHeader("apiKey")
	}

	if key == "" {
		resp := ginshared.GeneralResp{
			Succ:         false,
			ErrorCode:    "AuthFailed",
			ErrorMessage: "API Key missed",
		}
		c.JSON(401, resp)
		c.Abort()
		return
	}

	if id, ok := a.checkKey(key); ok {
		c.Set(KeyUser, id)
		c.Next()
	} else {
		resp := ginshared.GeneralResp{
			Succ:         false,
			ErrorCode:    "AuthFailed",
			ErrorMessage: "apiKey mismatched or been deleted",
		}

		c.JSON(http.StatusUnauthorized, resp)
		c.Abort()
	}
}

type AuthGroutRoute gin.IRoutes

func NewRouterRequiredAuth(group *gin.RouterGroup, auth *AuthService) AuthGroutRoute {
	result := group.Use(auth.Auth)
	return AuthGroutRoute(result)
}
