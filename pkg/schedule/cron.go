package schedule

import (
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type CronZaplog struct {
	logger *zap.Logger
}

func (c *CronZaplog) Info(msg string, keysAndValues ...interface{}) {
	args := []interface{}{msg}
	args = append(args, keysAndValues...)
	c.logger.Sugar().Info(args...)
}
func (c *CronZaplog) Error(err error, msg string, keysAndValues ...interface{}) {
	args := []interface{}{err.Error() + "," + msg}
	args = append(args, keysAndValues...)
	c.logger.Sugar().Error(args...)
}

func CreateSchedule(jobname, schedule string, cmd func(), logger *zap.Logger) (*cron.Cron, error) {
	l := &CronZaplog{
		logger: logger,
	}
	cr := cron.New(cron.WithChain(cron.Recover(l), cron.SkipIfStillRunning(l)))
	_, err := cr.AddFunc(schedule, cmd)
	if err != nil {
		return nil, err
	}
	cr.Start()

	entries := cr.Entries()
	nextRuntime := entries[0].Next

	logger.Debug("cron job scheduled", zap.String("job", jobname), zap.String("schedule", schedule), zap.Time("next", nextRuntime))

	return cr, nil
}
