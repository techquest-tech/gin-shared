//go:build !ram

package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type RedisConfig struct {
	Host    string
	Port    uint
	Account string
	Passwd  string
	Caroot  string // where is the ca pem file.
	Tls     bool   //TLS enabled ?
	DB      int    // 0: dev, 1: uat, 3: prd
}

func NewRedisClient(logger *zap.Logger) *redis.Client {
	opts := &redis.Options{}
	subRedis := viper.Sub("redis")
	cfg := &RedisConfig{
		Port: 6379,
	}
	if subRedis != nil {
		subRedis.Unmarshal(cfg)
	}
	if cfg.Host != "" {
		opts.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	}
	if cfg.Account != "" {
		opts.Username = cfg.Account
		opts.Password = cfg.Passwd
	}
	if cfg.DB > 0 {
		opts.DB = cfg.DB
	}

	if cfg.Tls {
		tconfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
			ServerName: cfg.Host,
		}
		logger.Info("TLS is enabled.")
		if cfg.Caroot != "" {
			caCert, err := os.ReadFile(cfg.Caroot)
			if err != nil {
				panic("read redis ca pem failed. " + err.Error())
			}
			caCertPool := x509.NewCertPool()
			if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
				panic("parse ca certs failed.")
			}
			tconfig.RootCAs = caCertPool
			logger.Info("redis RootCAs loaded done")
		}
		opts.TLSConfig = tconfig
	}

	client := redis.NewClient(opts)
	logger.Info("connected to redis", zap.String("redis", opts.Addr))
	return client
}

func NewCacheProvider[T any](t time.Duration) CacheProvider[T] {
	rr := &RedisProvider[T]{
		// prefix:  randstr.Hex(4) + "-",
		timeout: t,
	}
	tname := fmt.Sprintf("%T", rr)

	from := strings.LastIndexByte(tname, '.')
	from2 := strings.LastIndexByte(tname, '[')

	if from < from2 {
		from = from2
	}

	if from > 0 {
		from = from + 1
	}
	to := strings.LastIndexByte(tname, ']')

	rr.prefix = tname[from:to] + "-"

	err := core.GetContainer().Invoke(func(client *redis.Client) {
		rrr := cache.New(&cache.Options{
			Redis:      client,
			LocalCache: cache.NewTinyLFU(1000, t),
		})
		rr.cache = rrr
	})
	if err != nil {
		zap.L().Error("new cache provider failed.", zap.Error(err))
		// panic(err)
	}
	return rr
}

type RedisProvider[T any] struct {
	prefix  string
	timeout time.Duration
	cache   *cache.Cache
}

// Get implements CacheProvider.
func (r *RedisProvider[T]) Get(key string) (T, bool) {
	var value T
	if r.cache == nil {
		zap.L().Warn("redis cache is not functional now, ")
		return value, false
	}
	k := r.prefix + key
	zap.L().Debug("try to read value from redis", zap.String("key", k))

	ctx := context.TODO()
	if r.cache.Exists(ctx, k) {
		t := reflect.TypeOf(value)
		if t.Kind() != reflect.Ptr {
			err := r.cache.Get(context.TODO(), k, &value)
			if err != nil {
				zap.L().Error("read redis cache failed.", zap.Error(err))
				return value, false
			}
			zap.L().Debug("read value from redis done", zap.String("key", k), zap.Any("value", value))
			return value, true
		} else {
			var vv *T
			err := r.cache.Get(context.TODO(), k, &vv)
			if err != nil {
				zap.L().Error("read redis cache failed.", zap.Error(err))
				return value, false
			}
			r := *vv
			zap.L().Debug("read value from redis done", zap.String("key", k), zap.Any("value", r))
			return r, true
		}
	}
	zap.L().Debug("no value from redis", zap.String("key", k))
	return value, false
}

type CacheWithTTL interface {
	GetTTL() time.Duration
}

func (r *RedisProvider[T]) setAny(key string, value any) {
	tt := r.timeout
	if v, ok := value.(CacheWithTTL); ok {
		tt = v.GetTTL()
		zap.L().Info("set value with ttl", zap.Duration("ttl", tt))
	}
	err := r.cache.Set(&cache.Item{
		Ctx:   context.TODO(),
		Key:   r.prefix + key,
		Value: value,
		TTL:   tt,
	})
	if err != nil {
		zap.L().Error("set cache failed.", zap.Error(err))
	}
	zap.L().Info("set cache done", zap.String("key", r.prefix+key))
}

// Set implements CacheProvider.
func (r *RedisProvider[T]) Set(key string, value T) {
	r.setAny(key, value)
}

func init() {
	core.Provide(NewRedisClient)
}
