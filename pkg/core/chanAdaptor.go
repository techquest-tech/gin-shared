package core

import (
	"sync"

	"github.com/thanhpk/randstr"
	"go.uber.org/zap"
)

// Even Bus is good, but can't controll conconcurreny well and performance is not as good as chan,
// target to replace all eventbus with chanAdaptor.
type ChanAdaptor[T any] struct {
	sender    chan T
	receivers map[string]chan T
	locker    sync.Mutex
}

type Handler[T any] func(data T)

func NewChanAdaptor[T any](buf int) *ChanAdaptor[T] {
	if buf == 0 {
		buf = 1
	}
	return &ChanAdaptor[T]{
		sender:    make(chan T, buf),
		receivers: make(map[string]chan T),
		locker:    sync.Mutex{},
	}
}

func (ca *ChanAdaptor[T]) Push(data T) {
	ca.sender <- data
}

func (ca *ChanAdaptor[T]) Subscripter(receiver string, fn Handler[T]) chan T {
	ca.locker.Lock()
	defer ca.locker.Unlock()
	if receiver == "" {
		receiver = randstr.Hex(16)
	}
	if t, ok := ca.receivers[receiver]; ok {
		return t
	}
	c := make(chan T)
	ca.receivers[receiver] = c
	zap.L().Info("receiver suscribed", zap.String("receiver", receiver))

	if fn != nil {
		go func() {
			for v := range c {
				fn(v)
			}
		}()
	}

	return c
}

func (ca *ChanAdaptor[T]) Start() {
	zap.L().Info("chanAdaptor started")
	for v := range ca.sender {
		for _, c := range ca.receivers {
			c <- v
		}
	}

	for _, c := range ca.receivers {
		close(c)
	}
	zap.L().Info("chanAdaptor stopped")
}

var ErrorAdaptor = NewChanAdaptor[error](1000)
