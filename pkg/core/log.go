package core

import (
	"os"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// func defaultLoggerSettings() *zap.Logger {
// 	return zap.NewExample()
// }

func InitLogger(p Bootup) (*zap.Logger, error) {

	err := InitConfig(p)
	if err != nil {
		return nil, err
	}

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

	config := zap.NewDevelopmentConfig()

	switch env {
	case "prod", "prd", "uat":
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	default:
		config = zap.NewDevelopmentConfig()
	}

	config.OutputPaths = []string{"stdout"}

	config.Level.SetLevel(level.Level())

	if !settings.GetBool("trace") {
		config.DisableStacktrace = true
	}

	//check if rotate enabled.
	// if settings.GetBool("rotate") {

	// 	settings.SetDefault("max", 32)
	// 	settings.SetDefault("backup", 30)
	// 	settings.SetDefault("age", 30)
	// 	settings.SetDefault("file", fmt.Sprintf("data/logs/%s.log", AppName))

	// 	rotateConfig := lumberjack.Logger{
	// 		Filename:   settings.GetString("file"),
	// 		MaxSize:    settings.GetInt("max"),
	// 		MaxBackups: settings.GetInt("backup"),
	// 		MaxAge:     settings.GetInt("age"),
	// 		Compress:   true,
	// 	}

	// 	rotate := func(e zapcore.Entry) error {
	// 		rotateConfig.Write([]byte(fmt.Sprintf("%+v\n", e)))
	// 		return nil
	// 	}

	// 	log.Print("rotate is enabled, to file " + rotateConfig.Filename)

	// 	return config.Build(zap.Hooks(rotate))
	// }
	l, err := config.Build()
	if err != nil {
		return nil, err
	}

	l.Debug("init logger done, and replace globals.")
	zap.ReplaceGlobals(l)

	return l, nil
}
