package schedule

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Step struct {
	Name           string
	Run            func() (any, error)
	RunWithContext func(ctx context.Context) (any, error)
}

type StepResult struct {
	Name     string
	Result   any
	Err      error
	Duration time.Duration
}

type StepError struct {
	Step string
	Err  error
}

func (e StepError) Error() string {
	if e.Step == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Step, e.Err)
}

func (e StepError) Unwrap() error {
	return e.Err
}

func RunSteps(steps ...Step) ([]StepResult, error) {
	return RunStepsWithContext(context.Background(), steps...)
}

func RunStepsWithContext(ctx context.Context, steps ...Step) ([]StepResult, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	logger := zap.L()

	results := make([]StepResult, 0, len(steps))
	var errs []error

	for i := range steps {
		step := steps[i]
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("step[%d]", i)
		}

		if ctx != nil && ctx.Err() != nil {
			err := ctx.Err()
			logger.Warn("step canceled before start", zap.String("step", name), zap.Int("index", i), zap.Error(err))
			results = append(results, StepResult{
				Name:     name,
				Err:      err,
				Duration: 0,
			})
			errs = append(errs, StepError{Step: name, Err: err})
			continue
		}

		logger.Debug("step start", zap.String("step", name), zap.Int("index", i))
		start := time.Now()
		var (
			res any
			err error
		)

		runWithContext := step.RunWithContext
		run := step.Run
		if runWithContext == nil && run == nil {
			err = errors.New("nil step function")
		} else {
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
						logger.Error("step panic", zap.String("step", name), zap.Int("index", i), zap.Any("recover", r))
					}
				}()

				if runWithContext != nil {
					res, err = runWithContext(ctx)
				} else {
					res, err = run()
				}
			}()
		}

		dur := time.Since(start)
		if err != nil {
			logger.Warn("step failed", zap.String("step", name), zap.Int("index", i), zap.Duration("duration", dur), zap.Error(err))
		} else {
			logger.Debug("step done", zap.String("step", name), zap.Int("index", i), zap.Duration("duration", dur))
		}
		results = append(results, StepResult{
			Name:     name,
			Result:   res,
			Err:      err,
			Duration: dur,
		})

		if err != nil {
			errs = append(errs, StepError{Step: name, Err: err})
		}
	}

	return results, errors.Join(errs...)
}

func RunStepsAsync(ctx context.Context, steps ...Step) ([]StepResult, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	logger := zap.L()

	results := make([]StepResult, len(steps))
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	wg.Add(len(steps))
	for i := range steps {
		i := i
		step := steps[i]
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("step[%d]", i)
		}

		if ctx != nil && ctx.Err() != nil {
			err := ctx.Err()
			logger.Warn("step canceled before start", zap.String("step", name), zap.Int("index", i), zap.Error(err), zap.Bool("async", true))
			results[i] = StepResult{
				Name:     name,
				Err:      err,
				Duration: 0,
			}
			mu.Lock()
			errs = append(errs, StepError{Step: name, Err: err})
			mu.Unlock()
			wg.Done()
			continue
		}

		go func() {
			defer wg.Done()

			logger.Debug("step start", zap.String("step", name), zap.Int("index", i), zap.Bool("async", true))
			start := time.Now()
			var (
				res any
				err error
			)

			runWithContext := step.RunWithContext
			run := step.Run
			if runWithContext == nil && run == nil {
				err = errors.New("nil step function")
			} else {
				func() {
					defer func() {
						if r := recover(); r != nil {
							err = fmt.Errorf("panic: %v", r)
							logger.Error("step panic", zap.String("step", name), zap.Int("index", i), zap.Bool("async", true), zap.Any("recover", r))
						}
					}()

					if runWithContext != nil {
						res, err = runWithContext(ctx)
					} else {
						res, err = run()
					}
				}()
			}

			dur := time.Since(start)
			if err != nil {
				logger.Warn("step failed", zap.String("step", name), zap.Int("index", i), zap.Bool("async", true), zap.Duration("duration", dur), zap.Error(err))
			} else {
				logger.Debug("step done", zap.String("step", name), zap.Int("index", i), zap.Bool("async", true), zap.Duration("duration", dur))
			}
			results[i] = StepResult{
				Name:     name,
				Result:   res,
				Err:      err,
				Duration: dur,
			}

			if err != nil {
				mu.Lock()
				errs = append(errs, StepError{Step: name, Err: err})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	return results, errors.Join(errs...)
}
