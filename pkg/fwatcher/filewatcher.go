package fwatcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/dig"
	"go.uber.org/zap"
)

func init() {
	ginshared.GetContainer().Provide(NewWatchExcelFolder, ginshared.ControllerOptions)
}

const (
	KeyExcelFolder = "path"
	// ExcelFolder    = "data/excel"
)

var ErrorOpenFileFailed = errors.New("failed to open file")

var mu sync.RWMutex

var FileWatcheSettingKey = "files"

type FileAction interface {
	HandleFile(file string) error
}

// FileWatcher, not support recursion & idempotent yet.
type FilelWatcher struct {
	Logger        *zap.Logger
	Action        FileAction
	Recursive     bool
	Path          string
	Interval      time.Duration
	Included      []string
	Excluded      []string
	RetryDelay    time.Duration
	RetryAttempts uint
	Delete        bool
	DoneFolder    string
	ErrorFolder   string
}

// type FileAction func(file string) error

func (e *FilelWatcher) StartService(ctx context.Context) {
	e.Walk()

	switch {
	case e.Interval > time.Second:
		e.ScheduleWalk(ctx)
	default:
		e.StartWatcher(ctx)
	}
}

func (e *FilelWatcher) ScheduleWalk(ctx context.Context) {
	ticker := time.NewTicker(e.Interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				e.Logger.Info("timer stop")
				ticker.Stop()
			case <-ticker.C:
				e.Logger.Info("time event triggered.")
				e.Walk()
			}
		}
	}()
	e.Logger.Info("schedule walk job done. ", zap.Duration("interval", e.Interval))
}

func (e *FilelWatcher) StartWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		// errMsg := fmt.Sprintf("new watcher failed. %+v", err)
		// panic(errMsg)
		e.Logger.Error("new watcher failed", zap.Any("error", err))
		return err
	}

	e.Logger.Info("file watcher started. ", zap.String("path", e.Path))

	// defer watcher.Close()

	go func() {

	watchloop:
		for {
			select {
			case <-ctx.Done():
				e.Logger.Info("Job done. watcher existed. ", zap.Any("message", ctx.Err()))
				watcher.Close()

				break watchloop

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				e.Logger.Error("watcher file error, ", zap.Any("error", err))

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				e.Logger.Debug("received event:", zap.Any("event", event))

				fullfilename := event.Name

				stat, err := os.Stat(fullfilename)
				if err != nil {
					e.Logger.Warn("stat file return error",
						zap.String("file", fullfilename),
						zap.Any("error", err),
					)
				}

				switch {
				case stat.IsDir() && event.Op&fsnotify.Create == fsnotify.Create && e.Recursive:
					watcher.Add(fullfilename)
					e.Logger.Info("monitor new created folder", zap.String("folder", fullfilename))
				case event.Op&fsnotify.Create == fsnotify.Create,
					event.Op&fsnotify.Write == fsnotify.Write,
					event.Op&fsnotify.Chmod == fsnotify.Chmod:

					e.handleFile(fullfilename)
				default:
					e.Logger.Info("ignored event.", zap.Any("event", event))
				}
			}
		}
	}()

	err = watcher.Add(e.Path)
	if err != nil {
		// errMsg := fmt.Errorf("start watcher failed for folder %s, %+v", e.Path, err)
		// panic(errMsg)
		e.Logger.Error("add folder to watcher failed.",
			zap.String("folder", e.Path),
			zap.Any("error", err),
		)
	}
	if e.Recursive {
		filepath.Walk(e.Path, func(path string, info fs.FileInfo, err error) error {
			if info.IsDir() {
				err := watcher.Add(path)
				if err != nil {
					e.Logger.Error("add folder to watcher failed.",
						zap.String("folder", path),
						zap.Any("error", err),
					)
					return err
				}
				e.Logger.Info("watch folder done", zap.String("folder", path))
			}
			return nil
		})
	}

	e.Logger.Info("excel watcher is ready")
	return nil
}

func (e *FilelWatcher) Filter(file string) bool {
	filename := strings.ToLower(filepath.Base(file))
	logger := e.Logger.With(zap.String("file", filename))

	matched := (len(e.Included) == 0)
	// if len(e.Included) > 0 {
	for _, item := range e.Included {
		item = strings.ToLower(item)
		if m, err := filepath.Match(item, filename); err == nil && m {
			logger.Debug("file matched.", zap.String("item", item))
			matched = true
			break
		}
	}
	if !matched {
		logger.Info("file is not included")
		return false
	}

	for _, item := range e.Excluded {
		item = strings.ToLower(item)
		m, err := filepath.Match(item, filename)
		if err == nil && m {
			logger.Debug("file excluded.", zap.String("item", item))
			return false
		}
	}
	return true
}

func (e *FilelWatcher) Walk() {
	filepath.Walk(e.Path, func(path string, info fs.FileInfo, err error) error {
		if !e.Recursive {
			depth := strings.Count(path, "/") - strings.Count(e.Path, "/")
			if depth > 2 {
				e.Logger.Info("recursive disabled. ignore all sub folders")
				return filepath.SkipDir
			}
		}
		switch {
		case strings.HasPrefix(info.Name(), "."):
			e.Logger.Info("ignored hidden file.", zap.String("file", info.Name()))
		case info.IsDir():
			e.Logger.Debug("walk into sub folder", zap.String("sub folder", path))
		default:
			e.handleFile(path)
		}
		return nil
	})
}

func (e *FilelWatcher) handleFile(file string) {
	if !e.Filter(file) {
		e.Logger.Info("file is not included.", zap.String("file", file))
		return
	}
	mu.Lock()
	defer mu.Unlock()

	//check file before process
	if !FileExisted(file) {
		e.Logger.Info("file doesn't exist or has been handled.", zap.String("file", file))
		return
	}

	err := retry.Do(func() error {
		return e.Action.HandleFile(file)
	})

	if err != nil {
		e.Logger.Error("failed to process file", zap.String("file", file), zap.Any("error", err))
		if e.ErrorFolder != "" {
			mv(file, e.ErrorFolder, e.Logger)
		}
		return
	}
	if e.DoneFolder != "" {
		mv(file, e.DoneFolder, e.Logger)
		return
	}
	if e.Delete {
		err := os.Remove(file)
		if err != nil {
			e.Logger.Error("delete file failed.", zap.String("file", file), zap.Any("error", err))
			return
		}
		e.Logger.Info("delete file done", zap.String("file", file))
	}
}

type FileWatcherParams struct {
	dig.In
	Ctx        context.Context
	Logger     *zap.Logger
	Action     FileAction
	Idempotent *FileIdempotent `optional:"true"`
}

func NewWatchExcelFolder(p FileWatcherParams) ginshared.DiController {

	settings := viper.Sub(FileWatcheSettingKey)
	if settings.GetBool("disabled") {
		p.Logger.Info("file watcher is disabled.")
		return nil
	}
	filewatcher := &FilelWatcher{
		Logger: p.Logger.With(zap.String("service", "filewatcher")),
		Action: p.Action,
	}

	// settings.SetDefault(KeyExcelFolder, ExcelFolder)
	settings.SetDefault("retryDelay", 100*time.Microsecond)
	settings.SetDefault("retryAttempts", uint(30))

	// settings.SetDefault("doneFolder", "done")

	settings.Unmarshal(filewatcher)

	if len(filewatcher.Included) == 0 {
		filewatcher.Included = []string{"*.*"}
	}

	if len(filewatcher.Excluded) == 0 {
		filewatcher.Excluded = []string{"~*", ".*"}
	}

	//init folder for done or error
	if filewatcher.DoneFolder != "" {
		// folder := filepath.Join(filewatcher.Path, filewatcher.DoneFolder)
		os.MkdirAll(filewatcher.DoneFolder, 0755)
	}
	if filewatcher.ErrorFolder != "" {
		// folder := filepath.Join(filewatcher.Path, filewatcher.ErrorFolder)
		os.MkdirAll(filewatcher.ErrorFolder, 0755)
	}

	//init retry
	retry.DefaultAttempts = filewatcher.RetryAttempts
	retry.DefaultDelay = filewatcher.RetryDelay

	retry.DefaultRetryIf = func(err error) bool {
		return err == ErrorOpenFileFailed
	}

	filewatcher.StartService(p.Ctx)

	return filewatcher
}
