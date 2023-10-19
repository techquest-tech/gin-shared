package core

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

var embedcache = viper.New()

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

func ToEmbedConfig(content []byte) {
	if embedGenerated() {
		// zap.L().Debug("embed file generated. content should be migirated.")
		return
	}
	configItem := viper.New()
	configItem.SetConfigType("yaml")
	err := configItem.ReadConfig(bytes.NewReader(content))
	if err != nil {
		fmt.Printf("read embed config failed. %v", err)
	}
	cf := configItem.AllSettings()
	viper.MergeConfigMap(cf)
	embedcache.MergeConfigMap(cf)

	zap.L().Warn("process preconfig yaml done, might overwrite some settings.", zap.Any("keys", configItem.AllKeys()))

}

func GenerateEmbedConfigfile() error {
	return embedcache.SafeWriteConfigAs(filepath.Join(ConfigFolder, EmbedConfigFile+".yaml"))
}

func loadConfig(configname string) *viper.Viper {
	profileConfig := viper.New()
	profileConfig.SetConfigName(configname)
	profileConfig.SetConfigType("yaml")
	profileConfig.AddConfigPath(ConfigFolder)
	profileConfig.AddConfigPath(".")
	return profileConfig
}

func InitConfig(p Bootup) error {
	if embedGenerated() {
		embed := loadConfig(EmbedConfigFile)
		err := embed.ReadInConfig()
		if err != nil {
			return err
		}
		viper.MergeConfigMap(embed.AllSettings())
		log.Println("embed config loaded.")
	}

	configName := os.Getenv("APP_CONFIG")
	if configName == "" {
		configName = "app"
		log.Printf("user Config = %s", configName)
	}
	viperApp := loadConfig(configName)

	err := viperApp.ReadInConfig()
	if err != nil {
		log.Printf("WARN! read config failed. %+v", err)
	}
	viper.MergeConfigMap(viperApp.AllSettings())

	envfile := os.Getenv("ENV")
	if envfile != "" {
		profileConfig := loadConfig(envfile)
		err := profileConfig.ReadInConfig()
		if err != nil {
			err = fmt.Errorf("load env profile %s failed, %w", envfile, err)
			log.Println(err.Error())
			panic(err)
		}
		result := profileConfig.AllSettings()
		viper.MergeConfigMap(result)
		// logger.Debug("env profile loaded.", zap.Any("result", result), zap.String("env", envfile))
		log.Printf("env profile %s loaded", envfile)
	}

	log.Print("load config done.")
	return nil
}
