package storage

import (
	"os"
	"sync"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Release func()

type InitFs func(key string) (afero.Fs, Release, error)

var (
	NamedFsService = map[string]InitFs{}
	FsCacheEnabled = map[string]bool{
		"":      true,
		"local": true,
	}
	fsCache = sync.Map{}
)

func CreateFs(key string) (afero.Fs, Release, error) {
	// 检查是否有 Key 对应的配置
	if key == "" || !viper.IsSet(key) {
		key = "fileroot"
	}

	fstype := viper.GetString(key + ".type")
	shouldCacheResult := FsCacheEnabled[fstype]
	// 检查是否有缓存
	if cached, ok := fsCache.Load(key); ok && shouldCacheResult {
		zap.L().Debug("return fs result from cache", zap.String("key", key), zap.String("type", fstype))
		f := cached.(afero.Fs)
		r1, _ := fsCache.Load(key + ".release")
		return f, r1.(Release), nil
	}
	initFs, ok := NamedFsService[fstype]
	var fs afero.Fs
	var r Release
	var err error

	if !ok {
		zap.L().Info("storage type not found, user default local filesystem", zap.String("fstype", fstype))
		r = func() {}
		path := viper.GetString(key + ".path")
		if path != "" {
			fs = afero.NewBasePathFs(afero.NewOsFs(), path)
		} else {
			fs = afero.NewOsFs()
		}
	} else {
		fs, r, err = initFs(key)
		if err != nil {
			zap.L().Error("init fs error", zap.String("key", key), zap.Error(err))
			return nil, nil, err
		}
	}

	if shouldCacheResult {
		fsCache.Store(key, fs)
		fsCache.Store(key+".release", r)
	}

	return fs, r, nil
}

func EnsureDir(fs afero.Fs, dir string) error {
	if exists, err := afero.DirExists(fs, dir); !exists && err == nil {
		return fs.MkdirAll(dir, os.ModePerm)
	} else if err != nil {
		return err
	}
	return nil
}
