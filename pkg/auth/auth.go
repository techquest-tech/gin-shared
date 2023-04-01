package auth

import (
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/orm"
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
	orm.AppendEntity(&AuthKey{})
	ginshared.GetContainer().Provide(func(ap AuthServiceParam) *AuthService {
		authService := &AuthService{
			Db:     ap.DB,
			logger: ap.Logger,
		}
		authSetting := viper.Sub("auth")
		if authSetting != nil {
			authSetting.Unmarshal(authService)
		}
		// if viper.GetBool(ginshared.KeyInitDB) {
		// 	ap.DB.AutoMigrate(&AuthKey{})
		// }
		return authService
	})
	ginshared.GetContainer().Provide(NewDefaultAuthedRouter)
}

type AuthKey struct {
	gorm.Model
	ApiKey     string `gorm:"size:64"`
	Owner      string `gorm:"size:64"`
	Role       string `gorm:"size:64"`
	Remark     string `gorm:"size:64"`
	Suspend    bool
	Expiretion *time.Time
}

type AuthService struct {
	Db     *gorm.DB
	logger *zap.Logger
	Keys   []string
}

// const CheckSql = "SELECT id,remark from appusers a where a.IsDeleted = 0 and a.AppKey = ?"

func (a *AuthService) Validate(key string) (*AuthKey, bool) {

	for _, k := range a.Keys {
		if k == key {
			a.logger.Debug("use build-in key")
			return &AuthKey{
				Model: gorm.Model{
					ID: math.MaxInt32 - 1,
				},
				ApiKey: key,
				Owner:  core.AppName,
				Role:   "admin",
			}, true
		}
	}

	if a.Db == nil {
		a.logger.Warn("DB is not enabled for Auth service.")
		return nil, false
	}

	authkey := &AuthKey{}
	err := a.Db.First(authkey, "api_key = ?", key).Error

	if err != nil {
		a.logger.Error("sql query error", zap.Any("error", err))
		return nil, false
	}
	a.logger.Debug("found in DB", zap.Uint("userID", authkey.ID))

	if authkey.Suspend {
		a.logger.Error("apiKey has been suspend", zap.String("apiKey", key))
		return authkey, false
	}
	if authkey.Expiretion != nil && authkey.Expiretion.After(time.Now()) {
		a.logger.Error("apiKey is expired.", zap.String("apiKey", key), zap.Time("expiretion", *authkey.Expiretion))
		return authkey, false
	}
	a.logger.Info("validate apiKey done")
	return authkey, true
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

	if authkey, ok := a.Validate(key); ok {
		c.Set(KeyUser, authkey.ID)
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

func (a *AuthService) CreateUser(owner, remark string) (string, error) {
	u4 := uuid.New()
	key := AuthKey{
		ApiKey: u4.String(),
		Owner:  owner,
		Remark: remark,
	}
	err := a.Db.Save(key).Error
	if err != nil {
		return "", err
	}
	return key.ApiKey, nil
}

type AuthedGroutRoute gin.IRoutes

func NewDefaultAuthedRouter(route *gin.Engine, auth *AuthService, logger *zap.Logger) AuthedGroutRoute {
	return NewURIAuthedRouter(route, auth, logger, "")
}

func NewURIAuthedRouter(route *gin.Engine, auth *AuthService, logger *zap.Logger, uri string) AuthedGroutRoute {
	viper.SetDefault("baseUri", "/api/rfid")
	viper.SetDefault("replyCode", 503)

	fulluri := viper.GetString("baseUri")
	if uri != "" {
		fulluri = fulluri + "/" + uri
	}
	replyCode := viper.GetInt("replyCode")
	return NewAuthedRouter(route, auth, logger, fulluri, replyCode)
}

func NewAuthedRouter(route *gin.Engine, auth *AuthService, logger *zap.Logger, base string, replyCode int) AuthedGroutRoute {
	authed := route.Group(base).Use(auth.Auth)
	authed = authed.Use(ginshared.NewErrorReport(replyCode, logger))
	return authed
}

func NewRouterRequiredAuth(group *gin.RouterGroup, auth *AuthService) AuthedGroutRoute {
	result := group.Use(auth.Auth)
	return AuthedGroutRoute(result)
}
