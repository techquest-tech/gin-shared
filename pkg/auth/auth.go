package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"github.com/techquest-tech/gin-shared/pkg/orm"
	"github.com/thanhpk/randstr"
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

	core.GetContainer().Provide(func(ap AuthServiceParam) *AuthService {
		c := cache.New[*AuthKey]()
		authService := &AuthService{
			Db:        ap.DB,
			logger:    ap.Logger,
			userCache: c,
			HeaderKey: "apiKey",
		}
		authSetting := viper.Sub("auth")
		if authSetting != nil {
			authSetting.Unmarshal(authService)
		}
		// if viper.GetBool(ginshared.KeyInitDB) {
		// 	ap.DB.AutoMigrate(&AuthKey{})
		// }
		ap.Logger.Info("api key service inited.")
		return authService
	})
	ginshared.GetContainer().Provide(NewDefaultAuthedRouter)
}

type AuthKey struct {
	gorm.Model
	UserName   string `gorm:"size:32"`
	ApiKey     string `gorm:"size:64;unique"`
	Owner      string `gorm:"size:64"`
	Role       string `gorm:"size:64"`
	Remark     string `gorm:"size:64"`
	Suspend    bool
	Expiretion *time.Time
}

type AuthService struct {
	Db        *gorm.DB
	logger    *zap.Logger
	Keys      []string
	userCache *cache.Cache[*AuthKey]
	HeaderKey string
}

func Hash(rawKey string) string {
	hash := sha256.New()
	hash.Write([]byte(rawKey))

	hashed := hash.Sum(nil)

	return hex.EncodeToString(hashed)
}

// const CheckSql = "SELECT id,remark from appusers a where a.IsDeleted = 0 and a.AppKey = ?"

func (a *AuthService) Validate(key string) (*AuthKey, bool) {
	authkey, found := a.userCache.Get(key)

	if found {
		zap.L().Debug("authed apikey from cached. return true")
		return authkey, true
	}

	hashed := Hash(key)

	for index, k := range a.Keys {
		if k == hashed {
			a.logger.Debug("use build-in key(hashed)")
			owner := core.AppName
			if index := strings.IndexByte(hashed, '-'); index > -1 {
				owner = hashed[:index]
			}
			c := &AuthKey{
				Model: gorm.Model{
					ID: uint(index),
				},
				ApiKey: hashed,
				Owner:  owner,
				Role:   "admin",
			}
			a.userCache.Set(key, c)
			return c, true
		}
	}

	if a.Db == nil {
		a.logger.Warn("DB is not enabled for Auth service.")
		return nil, false
	}

	if !found {
		authkey = &AuthKey{}
		err := a.Db.First(authkey, "api_key = ?", hashed).Error

		if err != nil {
			a.logger.Error("sql query error", zap.Any("error", err))
			return nil, false
		}
		a.logger.Debug("found hashed key in DB", zap.Uint("userID", authkey.ID))
	}

	if authkey.Suspend {
		a.logger.Error("apiKey has been suspend", zap.String("apiKey", hashed))
		return authkey, false
	}
	if authkey.Expiretion != nil && authkey.Expiretion.Before(time.Now()) {
		a.logger.Error("apiKey is expired.", zap.String("apiKey", hashed), zap.Time("expiretion", *authkey.Expiretion))
		return authkey, false
	}
	a.logger.Info("validate apiKey done")

	a.userCache.Set(key, authkey)

	return authkey, true
}

func (a *AuthService) Auth(c *gin.Context) {
	key := ""
	switch c.Request.Method {
	case "GET":
		key = c.Query(a.HeaderKey)
	default:
		key = c.PostForm(a.HeaderKey)
	}
	if key == "" {
		key = c.GetHeader(a.HeaderKey)
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
		c.Set(KeyUser, authkey)
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

func (a *AuthService) CreateUser(owner, username, remark, rawKey string) (string, error) {
	u4 := rawKey
	if u4 == "" {
		u4 = randstr.String(32)
	}

	key := &AuthKey{
		ApiKey:   Hash(u4),
		UserName: username,
		Owner:    owner,
		Remark:   remark,
	}
	err := a.Db.Save(key).Error
	if err != nil {
		return "", err
	}
	return u4, nil
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
