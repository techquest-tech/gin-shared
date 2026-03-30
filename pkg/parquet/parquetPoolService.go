package parquet

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jinzhu/copier"
	"github.com/parquet-go/parquet-go"
	"github.com/samber/lo"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/messaging"
	"github.com/techquest-tech/gin-shared/pkg/storage"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

type ParquetSetting struct {
	FsKey           string // key for load FS settings
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
		BufferDur:       time.Minute * 30,
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
	// var err error
	// service.fs, err = core.CreateFs(setting.Folder)
	// if err != nil {
	// 	zap.L().Fatal("create fs failed", zap.Error(err))
	// }
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
	// var err error
	// service.fs, err = core.CreateFs(clonedSettings.Folder)
	// if err != nil {
	// 	zap.L().Fatal("create fs failed.", zap.Error(err))
	// }
	return service
}

type ParquetDataService struct {
	Setting *ParquetSetting
	Schema  *parquet.Schema
	Raw     chan any
	Filter  func(msg []any) []any
	// Event   PersistEvent
	// fs afero.Fs
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
	fs, release, err := storage.CreateFs(p.Setting.FsKey)
	if err != nil {
		zap.L().Error("create fs failed.", zap.Error(err))
		return "", err
	}
	defer release()

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
		err = storage.EnsureDir(fs, dir)
		if err != nil {
			return "", err
		}
	}
	f, err := fs.Create(filename)
	if err != nil {
		logger.Error("create parquet file failed.", zap.Error(err))
		return "", err
	}
	defer f.Close()

	sanitized := sanitizeMessagesBySchema(msgs, p.Schema)
	err = parquet.Write(f, sanitized, options...)
	if err != nil {
		logger.Error("failed to write parquet file", zap.Error(err))
		return "", err
	}
	logger.Info("write parquet file done.", zap.String("filename", filename))
	return filename, nil
}

func sanitizeMessagesBySchema(msgs []any, schema *parquet.Schema) []any {
	if schema == nil {
		return msgs
	}
	rootType := schema.GoType()
	sanitized := make([]any, 0, len(msgs))
	for _, msg := range msgs {
		sanitized = append(sanitized, sanitizeBySchema(msg, rootType))
	}
	return sanitized
}

func sanitizeBySchema(input any, rootType reflect.Type) any {
	if input == nil {
		return nil
	}
	v := reflect.ValueOf(input)
	switch {
	case v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Type() == rootType:
		sv, ok := sanitizeValueByType(v.Elem(), rootType)
		if !ok {
			return input
		}
		nv := reflect.New(v.Type().Elem())
		nv.Elem().Set(sv)
		return nv.Interface()
	case v.Type() == rootType:
		sv, ok := sanitizeValueByType(v, rootType)
		if !ok {
			return input
		}
		return sv.Interface()
	default:
		return input
	}
}

func sanitizeValueByType(v reflect.Value, expectedType reflect.Type) (reflect.Value, bool) {
	if !v.IsValid() {
		return v, false
	}
	switch expectedType.Kind() {
	case reflect.String:
		if v.Kind() != reflect.String {
			return v, false
		}
		raw := v.String()
		if utf8.ValidString(raw) {
			return v, true
		}
		return reflect.ValueOf(strings.ToValidUTF8(raw, "�")).Convert(v.Type()), true
	case reflect.Pointer:
		if v.IsNil() {
			return v, true
		}
		if v.Kind() != reflect.Pointer {
			return v, false
		}
		sv, ok := sanitizeValueByType(v.Elem(), expectedType.Elem())
		if !ok {
			return v, false
		}
		nv := reflect.New(v.Type().Elem())
		nv.Elem().Set(sv)
		return nv, true
	case reflect.Struct:
		if v.Kind() != reflect.Struct {
			return v, false
		}
		nv := reflect.New(v.Type()).Elem()
		nv.Set(v)
		for i := 0; i < expectedType.NumField(); i++ {
			field := expectedType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			av := v.FieldByName(field.Name)
			nvf := nv.FieldByName(field.Name)
			if !av.IsValid() || !nvf.IsValid() || !nvf.CanSet() {
				continue
			}
			sv, ok := sanitizeValueByType(av, field.Type)
			if ok {
				nvf.Set(sv)
			}
		}
		return nv, true
	case reflect.Slice:
		if v.Kind() != reflect.Slice {
			return v, false
		}
		if v.IsNil() {
			return v, true
		}
		nv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			sv, ok := sanitizeValueByType(v.Index(i), expectedType.Elem())
			if ok {
				nv.Index(i).Set(sv)
			} else {
				nv.Index(i).Set(v.Index(i))
			}
		}
		return nv, true
	case reflect.Array:
		if v.Kind() != reflect.Array {
			return v, false
		}
		nv := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			sv, ok := sanitizeValueByType(v.Index(i), expectedType.Elem())
			if ok {
				nv.Index(i).Set(sv)
			} else {
				nv.Index(i).Set(v.Index(i))
			}
		}
		return nv, true
	case reflect.Map:
		if v.Kind() != reflect.Map {
			return v, false
		}
		if v.IsNil() {
			return v, true
		}
		nv := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if expectedType.Key().Kind() == reflect.String {
				sk, ok := sanitizeValueByType(key, expectedType.Key())
				if ok {
					key = sk
				}
			}
			value, ok := sanitizeValueByType(iter.Value(), expectedType.Elem())
			if !ok {
				value = iter.Value()
			}
			nv.SetMapIndex(key, value)
		}
		return nv, true
	default:
		return v, true
	}
}

func (p *ParquetDataService) flushMessages(logger *zap.Logger, msgs []any, reason string) {
	if len(msgs) == 0 {
		return
	}
	if p.Filter != nil {
		msgs = p.Filter(msgs)
	}
	if len(msgs) == 0 {
		return
	}
	logger.Info("flush buffered messages", zap.String("reason", reason), zap.Int("len", len(msgs)))
	_, err := p.WriteMessages(msgs)
	if err != nil {
		logger.Error("flush buffered messages failed", zap.String("reason", reason), zap.Error(err))
	}
}

func (p *ParquetDataService) drainRemaining(idleWait time.Duration) []any {
	msgs := make([]any, 0)
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	for {
		select {
		case msg, ok := <-p.Raw:
			if !ok {
				return msgs
			}
			msgs = append(msgs, msg)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleWait)
		case <-timer.C:
			return msgs
		}
	}
}

func (p *ParquetDataService) Start(ctx context.Context) error {
	logger := zap.L().With(zap.String("service", "parquet-data-service"), zap.String("schema", p.Schema.GoType().Name()))

	if p.Setting.BufferDur == 0 {
		p.Setting.BufferDur = 30 * time.Minute
	}
	if p.Setting.BufferSize == 0 {
		p.Setting.BufferSize = 10000
	}

	logger.Info("startup for message")

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, flushing remaining messages")
		case <-done:
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, draining channel")
			msgs := p.drainRemaining(500 * time.Millisecond)
			p.flushMessages(logger, msgs, "context_done")
			return nil
		default:
		}

		msgs, bufferedLen, _, ok := lo.BufferWithTimeout(p.Raw, p.Setting.BufferSize, p.Setting.BufferDur)

		if p.Filter != nil {
			msgs = p.Filter(msgs)
			bufferedLen = len(msgs)
		}

		if bufferedLen == 0 && ok {
			continue
		}
		if bufferedLen > 0 {
			logger.Debug("start persist messages", zap.Int("len", bufferedLen))
			start := time.Now()
			fullname, err := p.WriteMessages(msgs)
			if err != nil {
				logger.Error("write message to parquet file failed.", zap.Error(err))
				continue
			}
			dd := time.Since(start)
			logger.Debug("persist message done.", zap.String("trunk file", fullname), zap.Duration("duration", dd), zap.Int("len", bufferedLen))
		}

		if !ok {
			logger.Info("all process done. exit.")
			break
		}
	}

	return nil

}
