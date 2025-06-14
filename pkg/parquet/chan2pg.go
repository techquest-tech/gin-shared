package parquet

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/lo"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
	"go.uber.org/zap"
)

// 生成文件名
func generateFileName(folder, timestampformt string) (string, error) {
	timestamp := time.Now().Format(timestampformt)
	result := fmt.Sprintf("%s/%s.parquet", folder, timestamp)

	// 获取文件所在的目录
	dir := filepath.Dir(result)

	// 判断目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 目录不存在，创建目录
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			zap.L().Error("failed to create directory for parquet file")
			return "", err
		}
		zap.L().Info("directory created", zap.String("dir", dir))
	}

	return result, nil
}

// setting to how to save parquet file
type Chan2Parquet[T any] struct {
	Folder     string
	Filename   string
	BufferSize int // settings for batch
	BufferDur  time.Duration
	Compress   string // should enabled compress
	cp         parquet.CompressionCodec
	Processor  string
	Event      PersistEvent[T]
}

// persist data to parquet file, done return full path or error
func (setting *Chan2Parquet[T]) persist(data []T) (string, error) {
	logger := zap.L().With(zap.String("file", setting.Filename))
	l := len(data)
	logger.Info("start persist messages", zap.Int("len", l))
	fullname, err := generateFileName(setting.Folder, setting.Filename)
	if err != nil {
		logger.Error("generate file name failed.", zap.Error(err))
		return "", err
	}

	fw, err := local.NewLocalFileWriter(fullname)
	if err != nil {
		logger.Error("create file writer failed.", zap.Error(err))
		return "", err
	}
	var v T
	pw, err := writer.NewParquetWriter(fw, v, 4)
	pw.CompressionType = setting.cp
	if err != nil {
		logger.Error("create parquet writer failed.", zap.Error(err))
		return "", fmt.Errorf("failed to create parquet writer: %v", err)
	}
	start := time.Now()
	for _, item := range data {
		err = pw.Write(item)
		if err != nil {
			logger.Error("write parquet failed.", zap.Error(err))
			return "", err
		}
	}

	if err = pw.WriteStop(); err != nil {
		logger.Error("write stop failed.", zap.Error(err))
		return "", err
	}
	if err = fw.Close(); err != nil {
		logger.Error("close file writer failed.", zap.Error(err))
		return "", err
	}
	dd := time.Since(start)
	logger.Info("persist message done.", zap.String("trunk file", fullname), zap.Duration("duration", dd), zap.Int("len", l))
	return fullname, nil
}

func (setting *Chan2Parquet[T]) Start(c chan T) error {
	if setting.Processor == "" {
		setting.Processor = "ChanParquet"
	}
	logger := zap.L().With(zap.String("processor", setting.Processor))
	if setting.Folder == "" {
		setting.Folder = "data"
	}
	if setting.Filename == "" {
		setting.Filename = setting.Processor + "_20060102T150405"
	}
	if setting.BufferDur == 0 {
		setting.BufferDur = 30 * time.Minute
	}
	if setting.BufferSize == 0 {
		setting.BufferSize = 100000
	}
	if setting.Compress == "" {
		setting.Compress = "GZIP"
	}
	if setting.Event == nil {
		setting.Event = &DefaultPersistEvent[T]{}
	}
	cp := parquet.CompressionCodec_UNCOMPRESSED
	if setting.Compress != "" {
		var err error
		cp, err = parquet.CompressionCodecFromString(setting.Compress)
		if err != nil {
			logger.Error("invalid compress type", zap.Error(err))
			return err
		}
	}
	setting.cp = cp
	for {
		logger.Info("start buffer message", zap.Int("buffer_size", setting.BufferSize), zap.Duration("buffer_dur", setting.BufferDur))

		msgs, len, _, ok := lo.BufferWithTimeout(c, setting.BufferSize, setting.BufferDur)

		if len == 0 && ok {
			logger.Debug("no message to persist. start new buffer durtion.")
			continue
		}
		if len > 0 {
			filename, err := setting.persist(msgs)
			if err != nil {
				logger.Error("persist failed.", zap.Error(err))
				setting.Event.OnPersistFailed(msgs, err)
				continue
			}
			setting.Event.OnPersistDone(msgs, filename)
		} else {
			logger.Info("no message to persist.")
		}

		if !ok {
			logger.Info("all process done. exit.")
			break
		}
	}

	return nil
}
