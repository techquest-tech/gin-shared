package core

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

var AppName = "RFID App"
var Version = "latest"

func InitConfig() {

	configName := os.Getenv("APP_CONFIG")
	if configName == "" {
		configName = "app"
		fmt.Printf("user Config = %s", configName)
	}

	viper.SetConfigName(configName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("/etc/gin")
	viper.AddConfigPath("$HOME/.gin")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		log.Printf("WARN! read config failed. %+v", err)
	}

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
}
