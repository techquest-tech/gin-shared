package schedule

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunSteps(t *testing.T) {
	called := make([]string, 0, 3)

	res, err := RunSteps(
		Step{
			Name: "ok",
			Run: func() (any, error) {
				called = append(called, "ok")
				return 123, nil
			},
		},
		Step{
			Name: "err",
			Run: func() (any, error) {
				called = append(called, "err")
				return nil, assertError("boom")
			},
		},
		Step{
			Name: "panic",
			Run: func() (any, error) {
				called = append(called, "panic")
				panic("bad")
			},
		},
	)

	if strings.Join(called, ",") != "ok,err,panic" {
		t.Fatalf("called=%v", called)
	}

	if len(res) != 3 {
		t.Fatalf("len(res)=%d", len(res))
	}

	if res[0].Err != nil || res[0].Result.(int) != 123 {
		t.Fatalf("step ok result=%v err=%v", res[0].Result, res[0].Err)
	}

	if res[1].Err == nil || res[2].Err == nil {
		t.Fatalf("expected step errors, got err1=%v err2=%v", res[1].Err, res[2].Err)
	}

	if err == nil {
		t.Fatalf("expected joined error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "err: boom") {
		t.Fatalf("joined error should contain step name and message, got=%s", msg)
	}
	if !strings.Contains(msg, "panic: panic: bad") && !strings.Contains(msg, "panic: bad") {
		t.Fatalf("joined error should contain panic, got=%s", msg)
	}
}

func TestRunStepsAsync(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		_, _ = RunStepsAsync(context.Background(),
			Step{
				Name: "s1",
				Run: func() (any, error) {
					started <- "s1"
					<-release
					return 1, nil
				},
			},
			Step{
				Name: "s2",
				Run: func() (any, error) {
					started <- "s2"
					<-release
					return 2, assertError("boom")
				},
			},
		)
		close(done)
	}()

	got := map[string]struct{}{}
	for len(got) < 2 {
		select {
		case s := <-started:
			got[s] = struct{}{}
		case <-time.After(150 * time.Millisecond):
			t.Fatalf("expected both steps started concurrently, got=%v", got)
		}
	}

	select {
	case <-done:
		t.Fatalf("should wait for release before returning")
	case <-time.After(80 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("should finish after release")
	}

	res, err := RunStepsAsync(context.Background(),
		Step{
			Name: "ok",
			Run: func() (any, error) { return 123, nil },
		},
		Step{
			Name: "err",
			Run: func() (any, error) { return nil, assertError("boom") },
		},
	)

	if len(res) != 2 {
		t.Fatalf("len(res)=%d", len(res))
	}
	if res[0].Err != nil || res[0].Result.(int) != 123 {
		t.Fatalf("step ok result=%v err=%v", res[0].Result, res[0].Err)
	}
	if res[1].Err == nil {
		t.Fatalf("expected step err")
	}
	if err == nil || !strings.Contains(err.Error(), "err: boom") {
		t.Fatalf("expected joined error with step name, got=%v", err)
	}
}

func TestRunStepsWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	called := make(chan string, 1)
	cancelled := make(chan struct{})

	go func() {
		<-called
		cancel()
		close(cancelled)
	}()

	res, err := RunStepsWithContext(ctx,
		Step{
			Name: "wait-cancel",
			RunWithContext: func(ctx context.Context) (any, error) {
				called <- "started"
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
		Step{
			Name: "should-not-run",
			Run: func() (any, error) {
				t.Fatalf("should-not-run executed")
				return nil, nil
			},
		},
	)

	<-cancelled

	if len(res) != 2 {
		t.Fatalf("len(res)=%d", len(res))
	}
	if res[0].Err == nil || res[0].Err != context.Canceled {
		t.Fatalf("expected step1 canceled, got=%v", res[0].Err)
	}
	if res[1].Err == nil || res[1].Err != context.Canceled {
		t.Fatalf("expected step2 canceled without running, got=%v", res[1].Err)
	}
	if err == nil {
		t.Fatalf("expected joined error")
	}
}

func TestRunStepsAsyncWithContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	resCh := make(chan []StepResult, 1)
	errCh := make(chan error, 1)

	go func() {
		res, err := RunStepsAsync(ctx,
			Step{
				Name: "s1",
				RunWithContext: func(ctx context.Context) (any, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			},
			Step{
				Name: "s2",
				RunWithContext: func(ctx context.Context) (any, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			},
		)
		resCh <- res
		errCh <- err
	}()

	cancel()

	select {
	case res := <-resCh:
		err := <-errCh

		if len(res) != 2 {
			t.Fatalf("len(res)=%d", len(res))
		}
		if res[0].Err != context.Canceled || res[1].Err != context.Canceled {
			t.Fatalf("expected both canceled, got err1=%v err2=%v", res[0].Err, res[1].Err)
		}
		if err == nil {
			t.Fatalf("expected joined error")
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("should finish after context cancel")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
