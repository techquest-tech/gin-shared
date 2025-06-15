package parquet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

var (
	Folder4FailedMsgs = "./data/.error"
)

func init() {
	core.ProvideStartup(func(logger *zap.Logger) core.Startup {
		if _, err := os.Stat(Folder4FailedMsgs); os.IsNotExist(err) {
			err := os.MkdirAll(Folder4FailedMsgs, os.ModePerm)
			if err != nil {
				logger.Error("failed to create directory for failed messages")
				return nil
			}
			logger.Info("directory created", zap.String("dir", Folder4FailedMsgs))
		}
		return nil
	})

}

// PersistEvent is the interface that defines the callback functions for parquet persistence.
type PersistEvent interface {
	OnPersistFailed(data []any, err error)
	OnPersistDone(data []any, filename string)
}

type DefaultPersistEvent struct {
}

func (d *DefaultPersistEvent) OnPersistFailed(data []any, failed error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		panic("persist failed, and  marshal failed." + failed.Error())
	}
	payload := fmt.Sprintf("{\"error\":\"%s\",\"data\":%s}", failed.Error(), string(jsonData))

	filename := filepath.Join(Folder4FailedMsgs, time.Now().Format("20060102T150405.json"))
	os.WriteFile(filename, []byte(payload), 0644)
	zap.L().Info("wrote failed to file done", zap.String("filename", filename))
}

func (d *DefaultPersistEvent) OnPersistDone(data []any, filename string) {
	zap.L().Info("wrote to file done", zap.String("filename", filename))
}
