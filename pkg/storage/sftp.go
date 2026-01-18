package storage

import (
	"fmt"
	"log"

	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	"github.com/spf13/afero/sftpfs"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type SftpSettings struct {
	Endpoint       string
	Account        string
	Password       string
	PrivateKeypath string
	Path           string
}

func init() {
	NamedFsService["sftp"] = InitSftpRoot
}

func InitSftpRoot(key string) (afero.Fs, Release, error) {
	logger := zap.L()
	settings := SftpSettings{}
	err := viper.UnmarshalKey(key, &settings)
	if err != nil {
		logger.Error("Failed to load SFTP settings", zap.Error(err))
		return nil, nil, err
	}
	if settings.Endpoint == "" {
		logger.Error("SFTP endpoint is empty")
		return nil, nil, fmt.Errorf("SFTP endpoint is emtpy")
	}
	var fs afero.Fs
	var config *ssh.ClientConfig
	switch {
	case settings.Account != "" && settings.Password != "":
		config = &ssh.ClientConfig{
			User: settings.Account,
			Auth: []ssh.AuthMethod{
				ssh.Password(settings.Password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		logger.Info("login sftp with username and password", zap.String("username", settings.Account))
	default:
		// fs, err = afero.NewSFTPFs(settings.Endpoint)
		return nil, nil, fmt.Errorf("PrivateKey not impl yet.")
	}

	sshClient, err := ssh.Dial("tcp", settings.Endpoint, config)
	if err != nil {
		logger.Error("Failed to connect to SFTP server", zap.Error(err), zap.String("endpoint", settings.Endpoint))
		return nil, nil, err
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		log.Fatalf("创建 SFTP client 失败: %v", err)
	}

	fn := func() {
		logger.Info("disconnect sftp/ssh")
		sftpClient.Close()
		sshClient.Close()
	}

	fs = afero.NewBasePathFs(
		sftpfs.New(sftpClient),
		settings.Path, // 可选，远程起始目录；留空 "" 则从用户家目录开始
	)
	logger.Info("connect to sftp done", zap.String("endpoint", settings.Endpoint), zap.String("rootFolder", settings.Path))
	return fs, fn, nil
}
