package parquet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jinzhu/copier"
	"github.com/parquet-go/parquet-go"
	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/messaging"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

type ParquetSetting struct {
	Folder          string
	FilenamePattern string
	BufferSize      int // settings for batch
	BufferDur       time.Duration
	Compress        string // should enabled compress
	// Processor  string
}

func DefaultParquetSetting() *ParquetSetting {
	return &ParquetSetting{
		Folder:          "data",
		FilenamePattern: "chunk_20060102T150405",
		BufferSize:      10 * 1000,
		BufferDur:       time.Second * 30,
		Compress:        "GZIP",
	}
}

func NewParquetDataService(setting *ParquetSetting, s *parquet.Schema) (*ParquetDataService, error) {
	return &ParquetDataService{
		Setting: setting,
		Raw:     make(chan any, setting.BufferSize),
		Schema:  s,
	}, nil
}

func NewParquetDataServiceBySchema(setting *ParquetSetting, ss *parquet.Schema, c chan any) *ParquetDataService {
	return &ParquetDataService{
		Setting: setting,
		Raw:     c,
		Schema:  ss,
	}
}

func NewParquetDataServiceT[T any](settings *ParquetSetting, filenamePattern string, c chan T) *ParquetDataService {
	clonedSettings := &ParquetSetting{}

	copier.CopyWithOption(clonedSettings, settings, copier.Option{IgnoreEmpty: true, DeepCopy: true})
	var data T

	clonedSettings.FilenamePattern = fmt.Sprintf(filenamePattern, core.GetStructNameOnly(data))

	return &ParquetDataService{
		Setting: clonedSettings,
		Raw:     core.ToAnyChan(c),
		Schema:  parquet.SchemaOf(data),
	}
}

type ParquetDataService struct {
	Setting *ParquetSetting
	Schema  *parquet.Schema
	Raw     chan any
	Filter  func(msg []any) []any
	Event   PersistEvent
}

// 生成文件名
func generateFileName(folder, timestampformt string) (string, error) {
	timestamp := time.Now().Format(timestampformt)

	sand := randstr.Hex(4) // just incase any concurrent write to same file
	result := fmt.Sprintf("%s/%s_%s.parquet", folder, timestamp, sand)

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

func (p *ParquetDataService) WriteMessages(msgs []any) (string, error) {
	logger := zap.L()

	filename, err := generateFileName(p.Setting.Folder, p.Setting.FilenamePattern)
	if err != nil {
		logger.Error("generate file name failed.", zap.Error(err))
		return "", err
	}
	defer func() {
		if err := recover(); err != nil {
			zap.L().Error("write parquet file failed.", zap.Any("err", err))
			messaging.AbandonedChan <- map[string]any{
				"error":    err,
				"consumer": "redis2parquet",
				"data":     msgs,
			}
		}
	}()
	err = parquet.WriteFile(filename, msgs, p.Schema)
	if err != nil {
		zap.L().Error("failed to write parquet file", zap.Error(err))
		return "", err
	}
	zap.L().Info("write parquet file done.", zap.String("filename", filename))
	return filename, nil
}

func (p *ParquetDataService) Start(ctx context.Context) error {
	logger := zap.L().With(zap.String("service", "parquet-data-service"))

	if p.Setting.BufferDur == 0 {
		p.Setting.BufferDur = 10 * time.Second
	}
	if p.Setting.BufferSize == 0 {
		p.Setting.BufferSize = 1000
	}

	logger.Info("startup for message")

	for {
		logger.Debug("start buffer message", zap.Int("buffer_size", p.Setting.BufferSize), zap.Duration("buffer_dur", p.Setting.BufferDur))

		msgs, bufferedLen, _, ok := lo.BufferWithTimeout(p.Raw, p.Setting.BufferSize, p.Setting.BufferDur)

		if p.Filter != nil {
			msgs = p.Filter(msgs)
			bufferedLen = len(msgs)
		}

		if bufferedLen == 0 && ok {
			logger.Debug("no message to persist. start new buffer durtion.")
			continue
		}
		if bufferedLen > 0 {
			logger.Info("start persist messages", zap.Int("len", bufferedLen))
			start := time.Now()
			fullname, err := p.WriteMessages(msgs)
			if err != nil {
				logger.Error("write message to parquet file failed.", zap.Error(err))
				// return err
				if p.Event != nil {
					p.Event.OnPersistFailed(msgs, err)
				}
				continue
			}
			if p.Event != nil {
				go p.Event.OnPersistDone(msgs, fullname)
			}
			dd := time.Since(start)
			logger.Info("persist message done.", zap.String("trunk file", fullname), zap.Duration("duration", dd), zap.Int("len", bufferedLen))
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
