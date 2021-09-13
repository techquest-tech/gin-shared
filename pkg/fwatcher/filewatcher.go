package fwatcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/techquest-tech/gin-shared/pkg/ginshared"
	"go.uber.org/zap"
)

func init() {
	ginshared.GetContainer().Provide(NewWatchExcelFolder, ginshared.ControllerOptions)
}

const (
	KeyExcelFolder = "path"
	ExcelFolder    = "data/excel"
)

var ErrorOpenFileFailed = errors.New("failed to open file")

var mu sync.RWMutex

// FileWatcher, not support recursion & idempotent yet.
type FilelWatcher struct {
	Logger        *zap.Logger
	Action        FileAction
	Path          string
	Interval      time.Duration
	Included      []string
	Excluded      []string
	RetryDelay    time.Duration
	RetryAttempts uint
	DoneFolder    string
	ErrorFolder   string
}

type FileAction func(file string) error

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

func (e *FilelWatcher) StartWatcher(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		errMsg := fmt.Sprintf("new watcher failed. %+v", err)
		panic(errMsg)
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

				switch {
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
		errMsg := fmt.Errorf("start watcher failed for folder %s, %+v", e.Path, err)
		panic(errMsg)
	}

	e.Logger.Info("excel watcher is ready")

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
	files, err := os.ReadDir(e.Path)
	if err != nil {
		e.Logger.Error("failed to walk folder")
		return
	}
	for _, file := range files {

		switch {
		case strings.HasPrefix(file.Name(), "."):
			e.Logger.Info("ignored hidden file.", zap.String("file", file.Name()))

		case file.IsDir():
			e.Logger.Debug("currently don't support sub folder. will be next version")

		default:
			fullpath := filepath.Join(e.Path, file.Name())
			e.handleFile(fullpath)
		}
	}
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
		return e.Action(file)
	})

	if err != nil {
		e.Logger.Error("failed to process file", zap.String("file", file), zap.Any("error", err))
		if e.ErrorFolder != "" {
			mv(file, e.ErrorFolder, e.Logger)
		}
	}
	if e.DoneFolder != "" {
		mv(file, e.DoneFolder, e.Logger)
	}
}

func NewWatchExcelFolder(ctx context.Context, logger *zap.Logger, action FileAction) ginshared.DiController {

	settings := viper.Sub("excel")
	if settings.GetBool("disabled") {
		logger.Info("excel watcher is disabled.")
		return nil
	}
	excelwatch := &FilelWatcher{
		Logger: logger.With(zap.String("service", "excelwatcher")),
		Action: action,
	}

	settings.SetDefault(KeyExcelFolder, ExcelFolder)
	settings.SetDefault("retryDelay", 100*time.Microsecond)
	settings.SetDefault("retryAttempts", uint(30))
	settings.SetDefault("doneFolder", "done")

	settings.Unmarshal(excelwatch)

	if len(excelwatch.Included) == 0 {
		excelwatch.Included = []string{"*.xlsx"}
	}
	if len(excelwatch.Excluded) == 0 {
		excelwatch.Excluded = []string{"~*", ".*"}
	}

	//init folder for done or error
	if excelwatch.DoneFolder != "" {
		folder := filepath.Join(excelwatch.Path, excelwatch.DoneFolder)
		os.MkdirAll(folder, 0755)
	}
	if excelwatch.ErrorFolder != "" {
		folder := filepath.Join(excelwatch.Path, excelwatch.ErrorFolder)
		os.MkdirAll(folder, 0755)
	}

	//init retry
	retry.DefaultAttempts = excelwatch.RetryAttempts
	retry.DefaultDelay = excelwatch.RetryDelay

	retry.DefaultRetryIf = func(err error) bool {
		return err == ErrorOpenFileFailed
	}

	excelwatch.StartService(ctx)

	return excelwatch
}
