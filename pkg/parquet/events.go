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
type PersistEvent[T any] interface {
	OnPersistFailed(data []T, err error)
	OnPersistDone(data []T, filename string)
}

type DefaultPersistEvent[T any] struct {
}

func (d *DefaultPersistEvent[T]) OnPersistFailed(data []T, failed error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		panic("persist failed, and  marshal failed." + failed.Error())
	}
	payload := fmt.Sprintf("{\"error\":\"%s\",\"data\":%s}", failed.Error(), string(jsonData))

	filename := filepath.Join(Folder4FailedMsgs, time.Now().Format(time.RFC3339)+".json")
	os.WriteFile(filename, []byte(payload), 0644)
	zap.L().Info("wrote failed to file done", zap.String("filename", filename))
}

func (d *DefaultPersistEvent[T]) OnPersistDone(data []T, filename string) {
	zap.L().Info("wrote to file done", zap.String("filename", filename))
}
