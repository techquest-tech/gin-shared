package schedule

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeHash 是内存版 cache.Hash，仅用于单测，避免依赖 redis。
type fakeHash struct {
	data map[string]map[string]any
}

func newFakeHash() *fakeHash {
	return &fakeHash{data: make(map[string]map[string]any)}
}

func (f *fakeHash) Existed(ctx context.Context, key string) (bool, error) {
	_, ok := f.data[key]
	return ok, nil
}

func (f *fakeHash) SetTTL(ctx context.Context, key string, ttl time.Duration) {}

func (f *fakeHash) GetValues(ctx context.Context, key string, fields ...string) ([]any, error) {
	result := make([]any, len(fields))
	for i, field := range fields {
		if m, ok := f.data[key]; ok {
			result[i] = m[field]
		}
	}
	return result, nil
}

func (f *fakeHash) SetValues(ctx context.Context, key string, values map[string]any) error {
	m, ok := f.data[key]
	if !ok {
		m = make(map[string]any)
		f.data[key] = m
	}
	for k, v := range values {
		m[k] = v
	}
	return nil
}

func (f *fakeHash) GetAll(ctx context.Context, key string) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range f.data[key] {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result, nil
}

func storeHistory(t *testing.T, p *JobHistoryProvider, job string, h JobHistory) {
	t.Helper()
	raw, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal job history: %v", err)
	}
	if err := p.Persister.SetValues(context.Background(), jobHistoryPersisterKey, map[string]any{job: string(raw)}); err != nil {
		t.Fatalf("set job history: %v", err)
	}
}

const testJobName = "HandleSyncWdtUnSalesStockOutInfo"

// 复现线上事故：作业注册调度时写下的记录（Start 零值 + Succeed=false）不能被当成
// 「上一次完成」，否则调用方会把增量同步的起点算到公元 1 年。
func TestGetLastDoneJobHistoryIgnoresScheduleRegistrationRecord(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}

	storeHistory(t, p, testJobName, JobHistory{
		Job:     testJobName,
		Cron:    "*/6 * * * *",
		Next:    time.Now().Add(6 * time.Minute),
		Succeed: false,
	})

	if got := p.GetLastDoneJobHistory(testJobName); got != nil {
		t.Fatalf("registration record must not be treated as done, got %+v", got)
	}
}

// Start 为零值的脏记录（即使 Succeed=true）同样不能返回。
func TestGetLastDoneJobHistoryIgnoresZeroStart(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}

	storeHistory(t, p, testJobName, JobHistory{Job: testJobName, Succeed: true})

	if got := p.GetLastDoneJobHistory(testJobName); got != nil {
		t.Fatalf("zero start must be ignored, got %+v", got)
	}
}

func TestGetLastDoneJobHistoryReturnsSucceededRecord(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}
	start := time.Now().Add(-time.Hour).Truncate(time.Second)

	storeHistory(t, p, testJobName, JobHistory{
		Job:      testJobName,
		Cron:     "*/6 * * * *",
		Start:    start,
		Finished: start.Add(time.Minute),
		Succeed:  true,
	})

	got := p.GetLastDoneJobHistory(testJobName)
	if got == nil {
		t.Fatal("succeeded record should be returned")
	}
	if !got.Start.Equal(start) {
		t.Fatalf("start mismatch: want %s got %s", start, got.Start)
	}
}

func TestGetLastDoneJobHistoryHandlesBadPayload(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}
	ctx := context.Background()

	if err := p.Persister.SetValues(ctx, jobHistoryPersisterKey, map[string]any{testJobName: "{not-json"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := p.GetLastDoneJobHistory(testJobName); got != nil {
		t.Fatalf("bad payload must be ignored, got %+v", got)
	}

	if err := p.Persister.SetValues(ctx, jobHistoryPersisterKey, map[string]any{testJobName: ""}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := p.GetLastDoneJobHistory(testJobName); got != nil {
		t.Fatalf("empty payload must be ignored, got %+v", got)
	}
}

func TestGetLastDoneJobHistoryNilWhenNoRecord(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}
	if got := p.GetLastDoneJobHistory(testJobName); got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

// 注册调度（UpsertJobSchedule）不能抹掉已完成记录的 Start/Succeed/Finished。
func TestUpsertJobScheduleKeepsDoneFields(t *testing.T) {
	p := &JobHistoryProvider{Persister: newFakeHash()}
	start := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	finished := start.Add(30 * time.Second)

	storeHistory(t, p, testJobName, JobHistory{
		Job:      testJobName,
		Start:    start,
		Finished: finished,
		Duration: finished.Sub(start),
		Succeed:  true,
	})

	next := time.Now().Add(6 * time.Minute).Truncate(time.Second)
	p.UpsertJobSchedule(testJobName, "*/6 * * * *", next, false)

	got := p.GetLastDoneJobHistory(testJobName)
	if got == nil {
		t.Fatal("done record must survive UpsertJobSchedule")
	}
	if !got.Start.Equal(start) || !got.Finished.Equal(finished) {
		t.Fatalf("done fields lost: %+v", got)
	}
	if !got.Next.Equal(next) || got.Cron != "*/6 * * * *" {
		t.Fatalf("schedule fields not refreshed: %+v", got)
	}
}
