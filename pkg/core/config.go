package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

const (
	EncryptedFile = "config/app.cfg"
)

var AppName = "RFID App"
var Version = "latest"
var ConfigFolder = "config"

// var EmbedConfigFile = "embed" // for init function can't guarantee embed config be load before startup, so write all content to file.

type ConfigSecret []byte

type EmbedConfigReady interface{}

type Bootup struct {
	dig.In
	Secret           ConfigSecret
	EmbedConfigReady EmbedConfigReady
}

var embedcache map[string]*viper.Viper = make(map[string]*viper.Viper)

// func embedGenerated() bool {
// 	_, err := os.Stat(filepath.Join(ConfigFolder, EmbedConfigFile+".yaml"))
// 	if err != nil {
// 		if !os.IsNotExist(err) {
// 			zap.L().Error("check file status failed.", zap.Error(err))
// 		}
// 		return false
// 	}
// 	return true
// }

var embedConfigLocker = sync.Mutex{}

func ToEmbedConfig(content []byte, keys ...string) {
	embedConfigLocker.Lock()
	defer embedConfigLocker.Unlock()

	configItem := viper.New()
	configItem.SetConfigType("yaml")
	err := configItem.ReadConfig(bytes.NewReader(content))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read embed config failed. %v", err)
		}
	}

	// embedcache.MergeConfigMap(cf)
	configKey := strings.Join(keys, "-")
	embed, ok := embedcache[configKey]
	if !ok {
		embed = configItem
	} else {
		cf := configItem.AllSettings()
		embed.MergeConfigMap(cf)
	}
	embedcache[configKey] = embed

	// if !embedGenerated() {
	// 	viper.MergeConfigMap(cf)
	// 	zap.L().Warn("process preconfig yaml done, might overwrite some settings.", zap.Any("keys", configItem.AllKeys()))
	// }
}

// make sure run it in main before init anything, just make sure, all embed config inited.
func InitEmbedConfig() {
	config, ok := embedcache[""]
	if ok {
		viper.MergeConfigMap(config.AllSettings())
		log.Printf("default embed config loaded.")
	} else {
		log.Printf("no embed config files at all.")
	}
	Provide(func() EmbedConfigReady { return nil })
}

// func GenerateEmbedConfigfile() error {
// 	for k, v := range embedcache {
// 		filename := EmbedConfigFile + ".yaml"
// 		if k != "" {
// 			filename = fmt.Sprintf("%s-%s.yaml", EmbedConfigFile, k)
// 		}
// 		err := v.WriteConfigAs(filepath.Join(ConfigFolder, filename))
// 		if err != nil {
// 			return err
// 		}
// 		zap.L().Info("write config file done", zap.String("configFile", filename))
// 	}
// 	if len(embedcache) == 0 {
// 		zap.L().Info("no config file generated.")
// 	}
// 	return nil
// }

func loadConfig(configname string) error {
	profileConfig := viper.New()
	profileConfig.SetConfigName(configname)
	profileConfig.SetConfigType("yaml")
	profileConfig.AddConfigPath(ConfigFolder)
	profileConfig.AddConfigPath(".")

	err := profileConfig.ReadInConfig()
	if err != nil {
		log.Printf("load %s failed. %v", configname, err)
		return err
	}
	viper.MergeConfigMap(profileConfig.AllSettings())
	log.Println(configname + " config loaded.")
	return nil
}

func InitConfig(p Bootup) error {
	if p.Secret != nil {
		err := ReadEncryptConfig(p.Secret, EncryptedFile)
		if err == nil {
			log.Printf("read from %s, load config done.\n", EncryptedFile)
			return nil
		}
	}

	// if embedGenerated() {
	// 	err := loadConfig(EmbedConfigFile)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	configName := os.Getenv("APP_CONFIG")
	if configName == "" {
		configName = "app"
		log.Printf("user Config = %s", configName)
	}

	err := loadConfig(configName)
	if err != nil {
		log.Printf("load config failed." + err.Error())
		// return err
	}
	envfile := os.Getenv("ENV")
	if envfile != "" {
		//load embed_envfile first
		// loadConfig(EmbedConfigFile + "-" + envfile) //allow config file missing
		envConfig := embedcache[envfile]
		if envConfig != nil {
			viper.MergeConfigMap(envConfig.AllSettings())
		}
		loadConfig(envfile)
	}

	log.Print("load config done.")
	return nil
}

func EncryptConfig() error {
	return GetContainer().Invoke(func(logger *zap.Logger, secret ConfigSecret) error {
		// tmp := "config/tmp.yaml"

		// err := viper.WriteConfigAs(tmp)
		// if err != nil {
		// 	logger.Error("read all settings failed.", zap.Error(err))
		// 	return err
		// }
		values := viper.AllSettings()
		raw, err := json.Marshal(values)
		if err != nil {
			return err
		}
		//encrypt it with AES
		out, err := Encrypt(secret, raw)
		if err != nil {
			return err
		}

		err = os.WriteFile(EncryptedFile, out, 0644)
		if err != nil {
			logger.Error("write encrypted file failed.", zap.Error(err))
			return err
		}

		zap.L().Info("config file encrypt", zap.String("toFile", EncryptedFile), zap.Int("len", len(out)))
		return nil
	})
}

func ReadEncryptConfig(secret []byte, toFile string) error {
	logger := zap.L()
	raw, err := os.ReadFile(toFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read encrypt file failed. error %v\n", err.Error())
		}
		return err
	}

	out, err := Decrypt(secret, raw)
	if err != nil {
		log.Printf("decrypt file failed. %v", err)
		return err
	}

	values := make(map[string]any)
	err = json.Unmarshal(out, &values)
	if err != nil {
		return err
	}

	viper.MergeConfigMap(values)

	// reader := bytes.NewReader(out)

	// viper.SetConfigType("yaml")

	// err = viper.ReadConfig(reader)
	// if err != nil {
	// 	logger.Error("read encrypted config failed.")
	// 	return err
	// }

	logger.Info("load encrypted config done")
	return nil
}

func init() {
	Provide(InitLogger)
}
