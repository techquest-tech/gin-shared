package fwatcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

func FileExisted(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}

func mv(file, subFolder string, logger *zap.Logger) {
	filename := filepath.Base(file)
	folder := filepath.Dir(file)

	nFolder := filepath.Join(folder, subFolder)

	// os.MkdirAll(nFolder, 0755)

	nFileName := filepath.Join(nFolder, filename)

	if FileExisted(nFileName) {
		timestamp := time.Now().Format("20060102T150405")
		nFileName = filepath.Join(nFolder, fmt.Sprintf("%s-%s", timestamp, filename))
	}

	err := os.Rename(file, nFileName)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Error("mv to done folder failed.", zap.Any("error", err))
		}
		return
	}
	logger.Info("mv to sub folder done", zap.String("target", nFileName))
}
