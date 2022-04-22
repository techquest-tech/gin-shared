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

var ErrorShouldRetry = errors.New("process failed but should retry")

// var mu sync.RWMutex
var cache sync.Map

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
	ShouldRetry   retry.RetryIfFunc
	Idempotent    *FileIdempotentService
}

func (e *FilelWatcher) isDoneOrErrFolder(filename string) bool {
	if e.DoneFolder != "" && strings.HasPrefix(filename, e.DoneFolder) {
		return true
	}
	if e.ErrorFolder != "" && strings.HasPrefix(filename, e.ErrorFolder) {
		return true
	}
	return false
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
		defer e.Logger.Info("stop watch files")
	watchloop:
		for {
			select {
			case <-ctx.Done():
				e.Logger.Info("Job done. watcher existed. ", zap.Any("message", ctx.Err()))
				watcher.Close()

				break watchloop

			case err, ok := <-watcher.Errors:
				e.Logger.Error("watcher file error, ", zap.Any("error", err))
				if !ok {
					continue
				}

			case event, ok := <-watcher.Events:
				if !ok {
					continue
				}
				e.Logger.Debug("received event:", zap.Any("event", event))

				fullfilename := event.Name

				switch {
				case e.isDoneOrErrFolder(fullfilename):
					e.Logger.Debug("ignored done or error folder")
					continue

				case event.Op&fsnotify.Create == fsnotify.Create:
					stat, err := os.Stat(fullfilename)
					if err != nil {
						e.Logger.Warn("stat file return error",
							zap.String("file", fullfilename),
							zap.Any("error", err),
						)
						continue
					}
					if stat.IsDir() {
						if e.Recursive {
							watcher.Add(fullfilename)
							e.Logger.Info("monitor new created folder", zap.String("folder", fullfilename))
						} else {
							e.Logger.Debug("ignored, recursive is disabled.")
						}
					} else {
						// it's file & should processing it.
						e.handleFile(fullfilename)
					}

				case event.Op&fsnotify.Write == fsnotify.Write,
					event.Op&fsnotify.Chmod == fsnotify.Chmod:

					e.handleFile(fullfilename)
				default:
					e.Logger.Debug("ignored event.", zap.Any("event", event))
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
		return err
	}
	if e.Recursive {
		filepath.Walk(e.Path, func(path string, info fs.FileInfo, err error) error {
			if e.isDoneOrErrFolder(path) {
				return filepath.SkipDir
			}
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

	if e.isDoneOrErrFolder(filename) {
		logger.Debug("it's under done or error folder, file ignored.")
		return false
	}

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
		logger.Debug("file is not included")
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
	logger.Debug("filed filered and it's included.")
	return true
}

func (e *FilelWatcher) Walk() {
	filepath.Walk(e.Path, func(path string, info fs.FileInfo, err error) error {
		switch {
		case strings.HasPrefix(info.Name(), "."):
			e.Logger.Debug("ignored hidden file.", zap.String("file", info.Name()))
		case e.isDoneOrErrFolder(path):
			e.Logger.Debug("ignored done/error folder", zap.String("file", path))
			return filepath.SkipDir
		case info.IsDir():
			switch {
			case path == e.Path:
				e.Logger.Debug("walk from root")
			case e.Recursive:
				e.Logger.Debug("walk into sub folder", zap.String("sub folder", path))
			default:
				e.Logger.Debug("recursive disabled. ignore all sub folders", zap.String("sub folder", path))
				return filepath.SkipDir
			}

		default:
			e.handleFile(path)
		}
		return nil
	})
}

func (e *FilelWatcher) handleFile(file string) {
	log := e.Logger.With(zap.String("file", file))

	if _, ok := cache.Load(file); ok {
		log.Info("file is processing.")
		return
	}

	cache.Store(file, true)
	defer cache.Delete(file)

	if !e.Filter(file) {
		e.Logger.Info("file is not included.", zap.String("file", file))
		return
	}

	if e.Idempotent != nil {
		result, err := e.Idempotent.Idempotent(file)
		if err != nil {
			e.Logger.Error("process file failed on Idempotent", zap.Error(err))
			return
		}
		if result {
			e.Logger.Info("file idempotent return true, file has been processed before.")
			return
		}
	}

	// mu.Lock()
	// defer mu.Unlock()

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
		e.Logger.Debug("going to mv file to done folder", zap.String("file", file))
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
	Idempotent *FileIdempotentService `optional:"true"`
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
	settings.SetDefault("retryDelay", 100*time.Millisecond)
	settings.SetDefault("retryAttempts", uint(30))

	// settings.SetDefault("doneFolder", "done")

	settings.Unmarshal(filewatcher)

	if len(filewatcher.Included) == 0 {
		filewatcher.Included = []string{"*.*"}
	}

	if len(filewatcher.Excluded) == 0 {
		filewatcher.Excluded = []string{"~*", ".*"}
	}

	if filewatcher.Path != "" {
		os.MkdirAll(filewatcher.Path, 0755)
		filewatcher.Logger.Info("touch folder", zap.String("watched folder", filewatcher.Path))
	}
	//init folder for done or error
	if filewatcher.DoneFolder != "" {
		// folder := filepath.Join(filewatcher.Path, filewatcher.DoneFolder)
		os.MkdirAll(filewatcher.DoneFolder, 0755)
		filewatcher.Logger.Info("touch done folder", zap.String("done folder", filewatcher.DoneFolder))
	}
	if filewatcher.ErrorFolder != "" {
		// folder := filepath.Join(filewatcher.Path, filewatcher.ErrorFolder)
		os.MkdirAll(filewatcher.ErrorFolder, 0755)
		filewatcher.Logger.Info("touch error folder", zap.String("error folder", filewatcher.ErrorFolder))
	}

	//init retry
	retry.DefaultAttempts = filewatcher.RetryAttempts
	retry.DefaultDelay = filewatcher.RetryDelay

	if filewatcher.ShouldRetry == nil {
		filewatcher.ShouldRetry = func(err error) bool {
			return errors.Is(err, ErrorShouldRetry) || errors.Is(err, &fs.PathError{})
			// return err == ErrorShouldRetry
		}
	}

	retry.DefaultRetryIf = filewatcher.ShouldRetry

	if !filewatcher.Delete && filewatcher.DoneFolder == "" {
		filewatcher.Idempotent = p.Idempotent
		filewatcher.Logger.Info("delete is false and done folder is empty. use Idempotent", zap.Any("Idempotent", p.Idempotent))
	} else {
		filewatcher.Logger.Debug("No Idempotent is needed.", zap.Bool("deleted", filewatcher.Delete), zap.String("donw folder", filewatcher.DoneFolder))
	}

	filewatcher.StartService(p.Ctx)

	return filewatcher
}
