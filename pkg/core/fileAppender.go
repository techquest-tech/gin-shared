package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

var DefaultFolder = "./data"

func AppendToFile[T any](c chan T, fileName string) {
	l := zap.L().With(zap.String("file", fileName))
	// openfile with append mode
	file, err := os.OpenFile(filepath.Join(DefaultFolder, fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		l.Error("open file failed", zap.Error(err))
		return
	}
	OnServiceStopping(func() {
		select {
		case _, ok := <-c:
			if !ok {
				l.Info("chan is closed.")
			}
		default:
			close(c)
			l.Info("close chan done.")
		}
	})

	defer file.Close()
	// c := adaptor.Sub("FileAppender_" + filename)
	for data := range c {
		payload, err := json.Marshal(data)
		if err != nil {
			l.Error("marshal data failed", zap.Error(err))
			continue
		}
		ts := time.Now().Format(time.RFC3339)
		_, err = file.WriteString(fmt.Sprintf("%s\t%s\n", ts, string(payload)))
		if err != nil {
			l.Error("write file failed", zap.Error(err))
		}
	}
	l.Info("done. file closed")
}
