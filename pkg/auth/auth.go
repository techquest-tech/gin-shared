package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
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
		// c := cache.New[*AuthKey]()
		authService := &AuthService{
			Db:     ap.DB,
			logger: ap.Logger,
			// userCache: c,
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
	UserName   string `gorm:"size:255"`
	ApiKey     string `gorm:"size:255;unique"`
	Owner      string `gorm:"size:255"`
	Role       string `gorm:"size:64"`
	Remark     string `gorm:"size:64"`
	Suspend    bool
	Expiretion *time.Time
}

type AuthService struct {
	Db     *gorm.DB
	logger *zap.Logger
	Keys   []string
	// userCache *cache.Cache[*AuthKey]
	HeaderKey string
}

type Owner struct {
	gorm.Model
	Ownername      string `gorm:"size:16;"`
	Suspended      bool
	DecoderVersion string `gorm:"size:16;"`
}

type StoreUser struct {
	UpdatedAt time.Time
	OwnerID   *uint  `gorm:"primaryKey"`
	StoreCode string `gorm:"size:64;primaryKey"`
	UserCode  string `gorm:"size:64;primaryKey"`
}

func Hash(rawKey string) string {
	hash := sha256.New()
	hash.Write([]byte(rawKey))

	hashed := hash.Sum(nil)

	return hex.EncodeToString(hashed)
}

// const CheckSql = "SELECT id,remark from appusers a where a.IsDeleted = 0 and a.AppKey = ?"

func (a *AuthService) Validate(key string) (*AuthKey, bool) {
	hashed := Hash(key)
	// authkey, found := a.userCache.Get(hashed)

	// if found {
	// 	zap.L().Debug("authed apikey from cached. return true")
	// 	return authkey, true
	// }

	for index, k := range a.Keys {
		if k == hashed {
			a.logger.Debug("use build-in key(hashed)")
			owner := ""
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
			// a.userCache.Set(hashed, c)
			return c, true
		}
	}

	if a.Db == nil {
		a.logger.Warn("DB is not enabled for Auth service.")
		return nil, false
	}

	// if !found {
	authkey := &AuthKey{}
	err := a.Db.First(authkey, "api_key = ?", hashed).Error

	if err != nil {
		a.logger.Error("sql query error", zap.Any("error", err))
		return nil, false
	}
	a.logger.Debug("found hashed key in DB", zap.Uint("userID", authkey.ID))
	// }

	if authkey.Suspend {
		a.logger.Error("apiKey has been suspend", zap.String("apiKey", hashed))
		return authkey, false
	}
	if authkey.Expiretion != nil && authkey.Expiretion.Before(time.Now()) {
		a.logger.Error("apiKey is expired.", zap.String("apiKey", hashed), zap.Time("expiretion", *authkey.Expiretion))
		return authkey, false
	}
	a.logger.Info("validate apiKey done")

	// a.userCache.Set(hashed, authkey)

	return authkey, true
}

func (a *AuthService) Auth(c *gin.Context) {
	key := c.Query(a.HeaderKey)

	if key == "" && c.Request.Method != "GET" {
		key = c.PostForm(a.HeaderKey)
	}
	// switch c.Request.Method {
	// case "GET":
	// 	key = c.Query(a.HeaderKey)
	// default:
	// 	key = c.PostForm(a.HeaderKey)
	// }
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
		c.Set("ownerID", authkey.ID)
		c.Set("ownerName", authkey.Owner)
		c.Set("user", authkey.UserName)
		c.Set("role", authkey.Role)
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

func (a *AuthService) CreateUser(owner, username, remark, rawKey, storeCode string) (string, error) {
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
	if storeCode != "" {
		ownerObj := &Owner{}
		err = a.Db.First(ownerObj, "ownername= ?", owner).Error
		if err != nil {
			return "", fmt.Errorf("%s is not found,pls add this owner first.error: %s", owner, err.Error())
		}
		storeUser := &StoreUser{}
		err = a.Db.First(storeUser, "owner_id =?  and user_code = ? and store_code = ?", ownerObj.ID, username, storeCode).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		storeUser.OwnerID = &ownerObj.ID
		storeUser.StoreCode = storeCode
		storeUser.UserCode = username
		err = a.Db.Save(storeUser).Error
		if err != nil {
			return "", err
		}
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
