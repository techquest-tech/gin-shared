package parquet

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func NewOssEventService(logger *zap.Logger) (PersistEvent, error) {
	result := &OssEventService{
		ID:     os.Getenv("OSS_ID"),
		Secret: os.Getenv("OSS_SECRET"),
	}
	err := viper.UnmarshalKey("oss", result)
	if err != nil {
		logger.Error("failed to unmarshal oss config", zap.Error(err))
		return nil, err
	}

	if result.Bucket == "" || result.ID == "" || result.Secret == "" {
		return nil, fmt.Errorf("OSS config missed")
	}

	client, err := oss.New(result.Endpoint, result.ID, result.Secret)
	if err != nil {
		return nil, err
	}
	result.client = client
	logger.Info("connect to oss success", zap.String("endpoint", result.Endpoint))
	return result, nil
}

type OssEventService struct {
	DefaultPersistEvent
	Endpoint  string
	ID        string
	Secret    string
	Bucket    string
	Prefix    string // prefix for object key
	Cleanup   bool
	ToOssFile func(filename string) string
	client    *oss.Client
}

func ToOssFile(filename string) string {
	objectKey := filename
	objectKey = strings.TrimPrefix(objectKey, "./")
	objectKey = strings.TrimPrefix(objectKey, "data/")
	return objectKey
}

func (d *OssEventService) OnPersistDone(data []any, filename string) {
	logger := zap.L().With(zap.String("filename", filename))

	logger.Info("going to upload file to oss", zap.String("OSS", d.Endpoint), zap.String("bucket", d.Bucket))

	// upload to oss
	bucket, err := d.client.Bucket(d.Bucket)
	if err != nil {
		// return err
		core.ErrorAdaptor.Push(core.ErrorReport{
			Error:     fmt.Errorf("failed to get bucket %s: %v", d.Bucket, err),
			Uri:       filename,
			HappendAT: time.Now(),
		})
		return
	}

	_, err = os.Stat(filename)
	if err != nil {
		core.ErrorAdaptor.Push(core.ErrorReport{
			Error:     fmt.Errorf("failed to get file info %s: %v", filename, err),
			Uri:       filename,
			HappendAT: time.Now(),
		})
		return
	}

	if d.ToOssFile == nil {
		d.ToOssFile = ToOssFile
	}

	objectKey := d.ToOssFile(filename)
	if d.Prefix != "" {
		objectKey = fmt.Sprintf("%s/%s", d.Prefix, objectKey)
	}

	err = bucket.UploadFile(objectKey, filename, 500*1024, oss.Routines(3))
	if err != nil {
		logger.Error("upload failed.", zap.Error(err))
		core.ErrorAdaptor.Push(core.ErrorReport{
			Error: fmt.Errorf("failed to get file info %s: %v", filename, err),
			Uri:   filename,
		})
		return
	}

	logger.Info("File uploaded to OSS", zap.String("objectKey", objectKey))
	if d.Cleanup {
		// delete file after uploaded
		err = os.Remove(filename)
		if err != nil {
			logger.Warn("clean up file failed.", zap.Error(err))
			return
		}
		logger.Info("clean up file done.")
	}
}
