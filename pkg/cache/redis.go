//go:build !ram

package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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

var DefaultLocalCacheItems = 0 //local cache items. it's important for performance & if redis failed.

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
		DefaultLocalCacheItems = subRedis.GetInt("localItem")
		logger.Info("load item value done", zap.Int("localItem", DefaultLocalCacheItems))
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

type CacheConfig struct {
	Disalbed   bool
	LocalItems int
	TTL        time.Duration
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

	prefix := tname[from:to]

	rr.prefix = prefix + "-"

	localItem := DefaultLocalCacheItems

	sub := viper.Sub("cache." + prefix)
	if sub != nil {
		cfg := &CacheConfig{}
		sub.Unmarshal(cfg)
		if cfg.Disalbed {
			zap.L().Info("cache disabled", zap.String("prefix", rr.prefix))
			return &DisabledCache[T]{prefix: rr.prefix}
		}
		// if cfg.LocalItems > 0 {
		zap.L().Info("local cache enabled", zap.String("prefix", rr.prefix))
		localItem = cfg.LocalItems
		// }
		if cfg.TTL > 0 {
			zap.L().Info("cache ttl set to", zap.String("prefix", rr.prefix), zap.Duration("ttl", cfg.TTL))
			rr.timeout = cfg.TTL
		}
	}

	err := core.GetContainer().Invoke(func(client *redis.Client) {
		opt := &cache.Options{
			Redis:     client,
			Marshal:   json.Marshal,
			Unmarshal: json.Unmarshal,
			// LocalCache: cache.NewTinyLFU(localItem, t),
		}
		if localItem > 0 {
			opt.LocalCache = cache.NewTinyLFU(localItem, t)
		}
		rrr := cache.New(opt)
		rr.cache = rrr
		rr.Client = client
	})
	if err != nil {
		zap.L().Error("new cache provider failed.", zap.Error(err))
		// panic(err)
	}
	return rr
}

type DisabledCache[T any] struct {
	prefix string
}

func (dd *DisabledCache[T]) Set(key string, value T) {
	zap.L().Debug("cache disabled", zap.String("prefix", dd.prefix))
}
func (dd *DisabledCache[T]) Keys() []string {
	zap.L().Debug("cache disabled", zap.String("prefix", dd.prefix))
	return []string{}
}
func (dd *DisabledCache[T]) Get(key string) (T, bool) {
	zap.L().Debug("cache disabled", zap.String("prefix", dd.prefix))
	var zero T
	return zero, false
}
func (dd *DisabledCache[T]) Del(key string) error {
	zap.L().Debug("cache disabled", zap.String("prefix", dd.prefix))
	return nil
}

type RedisProvider[T any] struct {
	prefix  string
	timeout time.Duration
	cache   *cache.Cache
	Client  *redis.Client
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
func (r *RedisProvider[T]) Keys() []string {
	raw, err := r.Client.Keys(context.TODO(), r.prefix+"*").Result()
	if err != nil {
		zap.L().Error("get keys failed.", zap.Error(err))
		return []string{}
	}
	keys := make([]string, 0)
	for _, v := range raw {
		k := strings.TrimPrefix(v, r.prefix)
		if k != "" {
			keys = append(keys, k)
		}

	}
	return keys
}
func (r *RedisProvider[T]) Del(key string) error {
	return r.cache.Delete(context.TODO(), r.prefix+key)
}

func init() {
	core.Provide(NewRedisClient)
}
