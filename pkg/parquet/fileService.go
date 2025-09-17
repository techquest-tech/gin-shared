package parquet

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	s3 "github.com/fclairamb/afero-s3"
	"github.com/spf13/afero"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

func CreateFs(path string) (afero.Fs, error) {
	l := zap.L().With(zap.String("path", path))
	// check if path included any tmpl, if yes, replace it shared vars
	if strings.Contains(path, "{{") {
		env, found := os.LookupEnv("ENV")
		if !found {
			env = "dev"
		}
		path = strings.ReplaceAll(path, "{{env}}", env)
		path = strings.ReplaceAll(path, "{{app}}", core.AppName)
		l.Debug("get real full path", zap.String("realpath", path))
	}

	switch {
	case path == "mem" || path == "":
		l.Info("use mem map fs")
		return afero.NewMemMapFs(), nil
	case strings.HasPrefix(path, "oss://"):
		ssopath := strings.TrimPrefix(path, "oss://")
		// ref to fsspec, oss://<bucket name>/<real path>
		parts := strings.SplitN(ssopath, "/", 2)
		if len(parts) < 2 {
			l.Error("invalid OSS URL: missing bucket or path")
			return nil, fmt.Errorf("invalid OSS URL: missing bucket or path")
		}

		bucket := parts[0]
		startPath := parts[1]
		accessKeyId := os.Getenv("OSS_ID")
		secretAccessKey := os.Getenv("OSS_SECRET")
		Endpoint := os.Getenv("OSS_ENDPOINT")
		region := os.Getenv("OSS_REGION")
		if accessKeyId == "" || secretAccessKey == "" || Endpoint == "" || region == "" {
			l.Warn("OSS config missed, use regular file instead", zap.String("path", startPath))
			return CreateFs(startPath)
		}
		l.Info("going to connect to oss", zap.String("endpoint", Endpoint), zap.String("bucket", bucket), zap.String("path", startPath))
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(accessKeyId, secretAccessKey, ""),
			Endpoint:    aws.String(Endpoint), // 关键：设置 OSS endpoint
		})
		if err != nil {
			log.Fatalf("Failed to create AWS session: %v", err)
		}

		// 2. 创建 Afero S3 文件系统
		s3Fs := s3.NewFs(bucket, sess)
		l.Info("s3Fs created")
		fs := afero.NewBasePathFs(s3Fs, startPath)
		return fs, nil
	default:
		l.Info("use os fs")
		bfs := afero.NewOsFs()
		err := EnsureDir(bfs, path)
		if err != nil {
			return nil, err
		}
		return afero.NewBasePathFs(bfs, path), nil
	}
}

func EnsureDir(fs afero.Fs, dir string) error {
	if exists, err := afero.DirExists(fs, dir); !exists && err == nil {
		return fs.MkdirAll(dir, os.ModePerm)
	} else if err != nil {
		return err
	}
	return nil
}
