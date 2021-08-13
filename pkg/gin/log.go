package gin

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// func defaultLoggerSettings() *zap.Logger {
// 	return zap.NewExample()
// }

func initLogger() (*zap.Logger, error) {

	InitConfig()

	settings := viper.Sub("log")

	if settings == nil {
		settings = viper.New()
	}

	settings.SetDefault("level", "info")

	//set the Level
	level := zap.NewAtomicLevel()
	level.UnmarshalText([]byte(settings.GetString("level")))

	env := strings.ToLower(os.Getenv("ENV"))

	if env == "" {
		env = settings.GetString("env")
	}

	var config zap.Config

	switch env {
	case "prod", "prd", "uat":
		config = zap.NewProductionConfig()

	default:
		config = zap.NewDevelopmentConfig()
	}

	config.Level.SetLevel(level.Level())

	//check if rotate enabled.
	if settings.GetBool("rotate") {

		settings.SetDefault("max", 32)
		settings.SetDefault("backup", 30)
		settings.SetDefault("age", 30)
		settings.SetDefault("file", fmt.Sprintf("data/logs/%s.log", AppName))

		rotateConfig := lumberjack.Logger{
			Filename:   settings.GetString("file"),
			MaxSize:    settings.GetInt("max"),
			MaxBackups: settings.GetInt("backup"),
			MaxAge:     settings.GetInt("age"),
			Compress:   true,
		}

		rotate := func(e zapcore.Entry) error {
			rotateConfig.Write([]byte(fmt.Sprintf("%+v\n", e)))
			return nil
		}

		log.Print("rotate is enabled, to file " + rotateConfig.Filename)

		return config.Build(zap.Hooks(rotate))
	}
	return config.Build()
}
