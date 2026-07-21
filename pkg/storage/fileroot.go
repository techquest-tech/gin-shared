package storage

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Release func()

const defaultStorageKey = "fileroot"

// FSFactory 定义按配置键创建文件系统的工厂函数。
// key: 存储配置键。
// 返回值：返回 afero.Fs、释放函数和错误信息。
type FSFactory func(key string) (afero.Fs, Release, error)

// PublicURLFunc 定义根据完整文件名生成公共访问 URL 的函数。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回公共访问 URL 和错误信息。
type PublicURLFunc func(fullFileName string) (string, error)

// PublicURLFuncFactory 定义按配置键返回公共 URL 生成函数的工厂函数。
// key: 存储配置键。
// 返回值：返回与当前 key 绑定的公共 URL 生成函数。
type PublicURLFuncFactory func(key string) PublicURLFunc

var (
	FSFactories            = map[string]FSFactory{}
	PublicURLFuncFactories = map[string]PublicURLFuncFactory{}
)

// CreateFs 根据配置键创建文件系统实例。
// key: 存储配置键，未配置时回退到默认 `fileroot`。
// 返回值：返回 afero.Fs、释放函数和错误信息。
func CreateFs(key string) (afero.Fs, Release, error) {
	// 检查是否有 Key 对应的配置
	if key == "" || !viper.IsSet(key) {
		key = defaultStorageKey
	}

	fstype := viper.GetString(key + ".type")
	initFs, ok := FSFactories[fstype]
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

	return fs, r, nil
}

// GetPublicURLInit 根据存储键返回公共 URL 生成函数。
// key: 存储配置键。
// 返回值：返回公共 URL 生成函数和错误信息。
func GetPublicURLFunc(key string) (PublicURLFunc, error) {
	logger := zap.L()
	if key == "" || !viper.IsSet(key) {
		key = defaultStorageKey
	}
	fstype := viper.GetString(key + ".type")
	initPublicURLFactory, ok := PublicURLFuncFactories[fstype]
	if !ok {
		initPublicURLFactory = PublicURLFuncFactories[""]
	}
	if initPublicURLFactory == nil {
		err := fmt.Errorf("public url initializer not found for storage type %q", fstype)
		logger.Error("[storage] get public url init failed",
			zap.String("key", key),
			zap.String("type", fstype),
			zap.Error(err),
		)
		return nil, err
	}

	logger.Info("[storage] get public url init done",
		zap.String("key", key),
		zap.String("type", fstype),
	)
	return initPublicURLFactory(key), nil
}

// GetPublicURLFuncDefault 返回默认存储键对应的公共 URL 生成函数。
// 返回值：返回默认 `fileroot` 配置对应的公共 URL 生成函数和错误信息。
func GetPublicURLFuncDefault() (PublicURLFunc, error) {
	return GetPublicURLFunc(defaultStorageKey)
}

// CreatePublicURLWithFunc 使用公共 URL 生成函数处理完整文件名。
// publicURLFunc: 已根据 key 解析得到的公共 URL 生成函数。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回公共访问 URL 和错误信息。
func CreatePublicURLWithFunc(publicURLFunc PublicURLFunc, fullFileName string) (string, error) {
	logger := zap.L()
	fullFileName = strings.TrimSpace(fullFileName)
	if fullFileName == "" {
		err := errors.New("full file name is empty")
		logger.Error("[storage] create public url failed", zap.String("fullFileName", fullFileName), zap.Error(err))
		return "", err
	}
	if publicURLFunc == nil {
		err := errors.New("public url func is nil")
		logger.Error("[storage] create public url failed",
			zap.String("fullFileName", fullFileName),
			zap.Error(err),
		)
		return "", err
	}

	logger.Info("[storage] create public url start",
		zap.String("fullFileName", fullFileName),
	)
	publicURL, err := publicURLFunc(fullFileName)
	if err != nil {
		logger.Error("[storage] create public url failed",
			zap.String("fullFileName", fullFileName),
			zap.Error(err),
		)
		return "", err
	}
	logger.Info("[storage] create public url done",
		zap.String("fullFileName", fullFileName),
		zap.String("publicURL", publicURL),
	)
	return publicURL, nil
}

// CreatePublicURL 根据完整文件名生成可公开访问的 URL。
// key: 存储配置键。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回公共访问 URL 和错误信息。
func CreatePublicURL(key string, fullFileName string) (string, error) {
	publicURLFunc, err := GetPublicURLFunc(key)
	if err != nil {
		return "", err
	}
	publicURL, err := CreatePublicURLWithFunc(publicURLFunc, fullFileName)
	if err != nil {
		return "", err
	}
	logger := zap.L()
	logger.Info("[storage] create public url by key done",
		zap.String("key", key),
		zap.String("fullFileName", fullFileName),
		zap.String("publicURL", publicURL),
	)
	return publicURL, nil
}

// CreatePublicURLDefault 使用默认存储键生成公共访问 URL。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回默认 `fileroot` 配置下的公共访问 URL 和错误信息。
func CreatePublicURLDefault(fullFileName string) (string, error) {
	publicURLFunc, err := GetPublicURLFuncDefault()
	if err != nil {
		return "", err
	}
	publicURL, err := CreatePublicURLWithFunc(publicURLFunc, fullFileName)
	if err != nil {
		return "", err
	}
	zap.L().Info("[storage] create public url by default key done",
		zap.String("key", defaultStorageKey),
		zap.String("fullFileName", fullFileName),
		zap.String("publicURL", publicURL),
	)
	return publicURL, nil
}

// GetDefaultPublicURLFunc 返回默认存储键对应的公共 URL 生成函数。
// Deprecated: 请使用 GetPublicURLFuncDefault。
// 返回值：返回默认 `fileroot` 配置对应的公共 URL 生成函数和错误信息。
func GetDefaultPublicURLFunc() (PublicURLFunc, error) {
	return GetPublicURLFuncDefault()
}

// CreateDefaultPublicURL 使用默认存储键生成公共访问 URL。
// Deprecated: 请使用 CreatePublicURLDefault。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回默认 `fileroot` 配置下的公共访问 URL 和错误信息。
func CreateDefaultPublicURL(fullFileName string) (string, error) {
	return CreatePublicURLDefault(fullFileName)
}

// GetPublicURLInit 根据存储键返回公共 URL 生成函数。
// Deprecated: 请使用 GetPublicURLFunc。
// key: 存储配置键。
// 返回值：返回公共 URL 生成函数和错误信息。
func GetPublicURLInit(key string) (PublicURLFunc, error) {
	return GetPublicURLFunc(key)
}

// CreatePublicURLWithInit 使用公共 URL 生成函数处理完整文件名。
// Deprecated: 请使用 CreatePublicURLWithFunc。
// publicURLFunc: 已根据 key 解析得到的公共 URL 生成函数。
// fullFileName: 完整文件名或完整文件路径。
// 返回值：返回公共访问 URL 和错误信息。
func CreatePublicURLWithInit(publicURLFunc PublicURLFunc, fullFileName string) (string, error) {
	return CreatePublicURLWithFunc(publicURLFunc, fullFileName)
}

func EnsureDir(fs afero.Fs, dir string) error {
	if exists, err := afero.DirExists(fs, dir); !exists && err == nil {
		return fs.MkdirAll(dir, os.ModePerm)
	} else if err != nil {
		return err
	}
	return nil
}
