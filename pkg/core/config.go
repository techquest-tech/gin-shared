package core

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

var AppName = "RFID App"
var Version = "latest"

// type ConfigYamlContent []byte

type Bootup struct {
	dig.In
	// EmbedConfig []ConfigYamlContent `group:"config"`
}

func ToEmbedConfig(content []byte) {
	configItem := viper.New()
	configItem.SetConfigType("yaml")
	err := configItem.ReadConfig(bytes.NewReader(content))
	if err != nil {
		fmt.Printf("read embed config failed. %v", err)
		// return err
	}
	viper.MergeConfigMap(configItem.AllSettings())
	zap.L().Warn("process preconfig yaml done, might overwrite some settings.", zap.Any("keys", configItem.AllKeys()))
}

func InitConfig(p Bootup) error {
	// for _, item := range p.EmbedConfig {
	// 	configItem := viper.New()
	// 	configItem.SetConfigType("yaml")
	// 	err := configItem.ReadConfig(bytes.NewReader(item))
	// 	if err != nil {
	// 		fmt.Printf("read embed config failed. %v", err)
	// 		return err
	// 	}
	// 	viper.MergeConfigMap(configItem.AllSettings())
	// }

	configName := os.Getenv("APP_CONFIG")
	if configName == "" {
		configName = "app"
		fmt.Printf("user Config = %s", configName)
	}
	viperApp := viper.New()
	viperApp.SetConfigName(configName)
	viperApp.SetConfigType("yaml")
	viperApp.AddConfigPath("config")
	viperApp.AddConfigPath("../config")
	viperApp.AddConfigPath("/etc/gin")
	viperApp.AddConfigPath("$HOME/.gin")
	viperApp.AddConfigPath(".")

	err := viperApp.ReadInConfig()
	if err != nil {
		log.Printf("WARN! read config failed. %+v", err)
	}
	viper.MergeConfigMap(viperApp.AllSettings())

	envfile := os.Getenv("ENV")
	if envfile != "" {
		profileConfig := viper.New()
		profileConfig.SetConfigName(envfile)
		profileConfig.SetConfigType("yaml")
		profileConfig.AddConfigPath("config")
		profileConfig.AddConfigPath("../config")
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
