package core

import (
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

// Even Bus is good, but can't controll conconcurreny well and performance is not as good as chan,
// target to replace all eventbus with chanAdaptor.
type ChanAdaptor[T any] struct {
	sender    chan T
	receivers map[string]chan T
	locker    sync.Mutex
	Started   bool
}

type Handler[T any] func(data T) error

func NewChanAdaptor[T any](buf int) *ChanAdaptor[T] {
	if buf == 0 {
		buf = 1
	}
	rr := &ChanAdaptor[T]{
		sender:    make(chan T, buf),
		receivers: make(map[string]chan T),
		locker:    sync.Mutex{},
	}
	OnServiceStarted(rr.Start)
	OnServiceStopping(func() {
		l := rr.getLogger()
		if rr == nil {
			l.Warn("chanAdaptor is nil, ignored.")
			return
		}
		close(rr.sender)
		l.Info("chanAdaptor stopped")
		time.Sleep(GraceShutdown)
	})
	return rr
}

func (ca *ChanAdaptor[T]) Push(data T) {
	ca.sender <- data
}

func (ca *ChanAdaptor[T]) getLogger() *zap.Logger {
	var v T
	return zap.L().With(zap.String("adaptor", GetStructNameOnly(v)))
}

func (ca *ChanAdaptor[T]) Sub(receiver string) chan T {
	if receiver == "" {
		receiver = randstr.Hex(16)
	}
	l := ca.getLogger().With(zap.String("receiver", receiver))
	if ca.Started {
		l.Warn("adaptor is started, can't add new receiver")
		return nil
	}
	ca.locker.Lock()
	defer ca.locker.Unlock()
	if _, ok := ca.receivers[receiver]; ok {
		// return t
		l.Warn("receiver already exists")
		return nil
	}
	c := make(chan T)
	ca.receivers[receiver] = c
	l.Info("receiver suscribed")
	return c
}

func (ca *ChanAdaptor[T]) Subscripter(receiver string, fn Handler[T]) {
	l := ca.getLogger().With(zap.String("receiver", receiver))
	if fn == nil {
		l.Warn("handler is nil")
		return
	}
	c := ca.Sub(receiver)
	if c == nil {
		return
	}
	go func() {
		for v := range c {
			err := fn(v)
			if err != nil {
				l.Error("handler error", zap.Error(err))
			}
		}
	}()
}

// make sure all receivers reg before start()
func (ca *ChanAdaptor[T]) Start() {
	l := ca.getLogger()
	if ca.Started {
		zap.L().Warn("chanAdaptor already started")
		return
	}
	l.Info("chanAdaptor started")
	ca.Started = true
	for v := range ca.sender {
		for receiver, c := range ca.receivers {
			c <- v
			l.Debug("chanAdaptor fwd message", zap.String("receiver", receiver))
		}
	}

	for _, c := range ca.receivers {
		close(c)
	}
	l.Info("chanAdaptor and receivers were stopped.")
}

func (ca *ChanAdaptor[T]) Stop() {
	ca.getLogger().Info("chanAdaptor stopping")
	close(ca.sender)
	ca.Started = false
}

func (ca *ChanAdaptor[T]) Receivers() []string {
	return lo.Keys(ca.receivers)
}

type ErrorReport struct {
	Uri       string
	FullStack []byte
	Error     error
	HappendAT time.Time
}

var ErrorAdaptor = NewChanAdaptor[ErrorReport](1000) // error adaptor for monitor error.
