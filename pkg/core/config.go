package core

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

var AppName = "RFID App"
var Version = "latest"
var ConfigFolder = "config"
var EmbedConfigFile = "embed" // for init function can't guarantee embed config be load before startup, so write all content to file.

type Bootup struct {
	dig.In
	// EmbedConfig []ConfigYamlContent `group:"config"`
}

var embedcache map[string]*viper.Viper = make(map[string]*viper.Viper)

func embedGenerated() bool {
	_, err := os.Stat(filepath.Join(ConfigFolder, EmbedConfigFile+".yaml"))
	if err != nil {
		if !os.IsNotExist(err) {
			zap.L().Error("check file status failed.", zap.Error(err))
		}
		return false
	}
	return true
}

func ToEmbedConfig(content []byte, keys ...string) {
	configItem := viper.New()
	configItem.SetConfigType("yaml")
	err := configItem.ReadConfig(bytes.NewReader(content))
	if err != nil {
		fmt.Printf("read embed config failed. %v", err)
	}
	cf := configItem.AllSettings()

	// embedcache.MergeConfigMap(cf)
	configKey := strings.Join(keys, "-")
	embed, ok := embedcache[configKey]
	if !ok {
		embed = configItem
	} else {
		embed.MergeConfigMap(cf)
	}
	embedcache[configKey] = embed

	if !embedGenerated() {
		viper.MergeConfigMap(cf)
		zap.L().Warn("process preconfig yaml done, might overwrite some settings.", zap.Any("keys", configItem.AllKeys()))
	}
}

func GenerateEmbedConfigfile() error {
	for k, v := range embedcache {
		filename := EmbedConfigFile + ".yaml"
		if k != "" {
			filename = fmt.Sprintf("%s-%s.yaml", EmbedConfigFile, k)
		}
		err := v.WriteConfigAs(filepath.Join(ConfigFolder, filename))
		if err != nil {
			return err
		}
		zap.L().Info("write config file done", zap.String("configFile", filename))
	}
	return nil
}

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
	if embedGenerated() {
		err := loadConfig(EmbedConfigFile)
		if err != nil {
			return err
		}
	}

	configName := os.Getenv("APP_CONFIG")
	if configName == "" {
		configName = "app"
		log.Printf("user Config = %s", configName)
	}

	err := loadConfig(configName)
	if err != nil {
		return err
	}
	envfile := os.Getenv("ENV")
	if envfile != "" {
		//load embed_envfile first
		loadConfig(EmbedConfigFile + "-" + envfile) //allow config file missing
		loadConfig(envfile)
	}

	log.Print("load config done.")
	return nil
}
