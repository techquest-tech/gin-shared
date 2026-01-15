package storage

import (
	"github.com/spf13/afero"
	"github.com/spf13/viper"
)

type Release func()

type InitFs func(key string) (afero.Fs, Release, error)

var NamedFsService = map[string]InitFs{}

func CreateFs(key string) (afero.Fs, Release, error) {
	fstype := viper.GetString(key + ".type")
	initFs, ok := NamedFsService[fstype]
	if !ok {
		return afero.NewOsFs(), func() {}, nil
	}
	return initFs(key)
}
