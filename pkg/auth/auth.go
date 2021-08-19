package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func init() {
	ginshared.GetContainer().Provide(func(db *gorm.DB, logger *zap.Logger) *AuthService {
		authService := &AuthService{
			Db:     db,
			logger: logger,
		}
		return authService
	})
}

type AuthKey struct {
	ID     uint
	Remark string
}

type AuthService struct {
	Db     *gorm.DB
	logger *zap.Logger
}

const CheckSql = "SELECT id,remark from appusers a where a.IsDeleted = 0 and a.AppKey = ?"

func (a *AuthService) checkKey(key string) bool {
	authkey := AuthKey{}
	err := a.Db.Raw(CheckSql, key).Scan(&authkey).Error

	if err != nil {
		a.logger.Error("sql query error", zap.Any("error", err))
		return false
	}

	return authkey.ID > 0
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
		resp := map[string]string{"error": "apiKey missed"}

		c.JSON(401, resp)
		c.Abort()
		return
	}

	if a.checkKey(key) {
		c.Next()
	} else {
		resp := map[string]string{"error": "apiKey mismatched or been deleted"}

		c.JSON(http.StatusUnauthorized, resp)
		c.Abort()
	}
}
