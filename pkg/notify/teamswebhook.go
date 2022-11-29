package notify

import "go.uber.org/zap"

type TeamsTmpl struct {
}

type TeamsNotify struct {
	Logger  *zap.Logger
	Webhook string
}
