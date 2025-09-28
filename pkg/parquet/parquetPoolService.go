package parquet

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jinzhu/copier"
	"github.com/parquet-go/parquet-go"
	"github.com/samber/lo"
	"github.com/spf13/afero"
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
	Ackfile         bool   // should generate ack file.
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

// func NewParquetDataService(setting *ParquetSetting, s *parquet.Schema) (*ParquetDataService, error) {
// 	return &ParquetDataService{
// 		Setting: setting,
// 		Raw:     make(chan any, setting.BufferSize),
// 		Schema:  s,
// 	}, nil
// }

func NewParquetDataServiceBySchema(setting *ParquetSetting, ss *parquet.Schema, c chan any) *ParquetDataService {
	service := &ParquetDataService{
		Setting: setting,
		Raw:     c,
		Schema:  ss,
	}
	var err error
	service.fs, err = core.CreateFs(setting.Folder)
	if err != nil {
		zap.L().Fatal("create fs failed", zap.Error(err))
	}
	return service
}

func NewParquetDataServiceT[T any](settings *ParquetSetting, filenamePattern string, c chan T) *ParquetDataService {
	clonedSettings := &ParquetSetting{}

	copier.CopyWithOption(clonedSettings, settings, copier.Option{IgnoreEmpty: true, DeepCopy: true})
	var data T

	clonedSettings.FilenamePattern = fmt.Sprintf(filenamePattern, core.GetStructNameOnly(data))

	// defaultEvent := &DefaultPersistEvent{
	// 	Ackfile: settings.Ackfile,
	// }

	service := &ParquetDataService{
		Setting: clonedSettings,
		Raw:     core.ToAnyChan(c),
		Schema:  parquet.SchemaOf(data),
		// Event:   defaultEvent,
	}
	var err error
	service.fs, err = core.CreateFs(clonedSettings.Folder)
	if err != nil {
		zap.L().Fatal("create fs failed.", zap.Error(err))
	}
	return service
}

type ParquetDataService struct {
	Setting *ParquetSetting
	Schema  *parquet.Schema
	Raw     chan any
	Filter  func(msg []any) []any
	// Event   PersistEvent
	fs afero.Fs
}

// 生成文件名
func generateFileName(_, timestampformt string) (string, error) {
	timestamp := time.Now().Format(timestampformt)

	sand := randstr.Hex(4) // just incase any concurrent write to same file
	result := fmt.Sprintf("%s_%s.parquet", timestamp, sand)

	// 获取文件所在的目录
	// dir := filepath.Dir(result)

	// // 判断目录是否存在
	// if _, err := os.Stat(dir); os.IsNotExist(err) {
	// 	// 目录不存在，创建目录
	// 	err := os.MkdirAll(dir, os.ModePerm)
	// 	if err != nil {
	// 		zap.L().Error("failed to create directory for parquet file")
	// 		return "", err
	// 	}
	// 	zap.L().Info("directory created", zap.String("dir", dir))
	// }

	return result, nil
}

func (p *ParquetDataService) WriteMessages(msgs []any) (string, error) {

	filename, err := generateFileName(p.Setting.Folder, p.Setting.FilenamePattern)
	if err != nil {
		zap.L().Error("generate file name failed.", zap.Error(err))
		return "", err
	}
	logger := zap.L().With(zap.String("filename", filename))

	defer func() {
		if err := recover(); err != nil {
			logger.Error("write parquet file failed.", zap.Any("err", err))
			messaging.AbandonedChan <- map[string]any{
				"error":    err,
				"consumer": "redis2parquet",
				"data":     msgs,
			}
		}
	}()
	options := []parquet.WriterOption{
		p.Schema,
	}

	if p.Setting.Compress != "" {
		options = append(options, parquet.Compression(p.Schema.Compression()))
	}

	dir := filepath.Dir(filename)
	if dir != "" {
		err = core.EnsureDir(p.fs, dir)
		if err != nil {
			return "", err
		}
	}
	f, err := p.fs.Create(filename)
	if err != nil {
		logger.Error("create parquet file failed.", zap.Error(err))
		return "", err
	}
	defer f.Close()

	err = parquet.Write(f, msgs, options...)
	if err != nil {
		logger.Error("failed to write parquet file", zap.Error(err))
		return "", err
	}
	logger.Info("write parquet file done.", zap.String("filename", filename))
	return filename, nil
}

func (p *ParquetDataService) Start(ctx context.Context) error {
	logger := zap.L().With(zap.String("service", "parquet-data-service"), zap.String("schema", p.Schema.GoType().Name()))

	if p.Setting.BufferDur == 0 {
		p.Setting.BufferDur = time.Minute
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
				// if p.Event != nil {
				// 	p.Event.OnPersistFailed(msgs, err)
				// }
				continue
			}
			// if p.Event != nil {
			// 	go p.Event.OnPersistDone(msgs, fullname)
			// }
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
