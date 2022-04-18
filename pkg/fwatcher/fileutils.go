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

func mv(file, targetFolder string, logger *zap.Logger) {
	filename := filepath.Base(file)
	// folder := filepath.Dir(file)

	// nFolder := filepath.Join(folder, targetFolder)
	// nFolder := filepath.Join(folder, subFolder)
	nFileName := filepath.Join(targetFolder, filename)

	if FileExisted(nFileName) {
		timestamp := time.Now().Format("20060102T150405")
		nFileName = filepath.Join(targetFolder, fmt.Sprintf("%s-%s", timestamp, filename))
	}

	logger.Debug("mv file", zap.String("src", file), zap.String("target", nFileName))

	err := os.Rename(file, nFileName)
	if err != nil {
		logger.Error("mv file error", zap.Error(err))
		if !errors.Is(err, os.ErrNotExist) {
			logger.Error("mv to done folder failed.", zap.Any("error", err))
		}

		return
	}
	logger.Info("mv to folder done", zap.String("target", nFileName))
}
