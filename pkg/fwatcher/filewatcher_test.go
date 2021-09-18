package fwatcher_test

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestWalk(t *testing.T) {
	rootpath := "D:\\weiyun"
	filepath.WalkDir(rootpath, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			// fmt.Printf("%s, %s,\n", path, d.Name(), d.IsDir())
			fmt.Println(path)
		}
		return nil
	})

	filepath.Walk(rootpath, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			fmt.Println(path)
		}
		return nil
	})
}
