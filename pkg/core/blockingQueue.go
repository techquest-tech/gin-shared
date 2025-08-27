package core

import (
	"errors"
	"sync"

	"go.uber.org/zap"
)

var ErrQueueClosed = errors.New("queue has been closed")

type BlockingQueue[T any] struct {
	mu     sync.Mutex
	cond   *sync.Cond
	data   []T
	closed bool
}

func NewBlockingQueue[T any]() *BlockingQueue[T] {
	q := &BlockingQueue[T]{}
	q.cond = sync.NewCond(&q.mu)
	q.data = make([]T, 0)
	OnServiceStopping(q.Close)
	return q
}

// Push 向队列推入元素，不会阻塞（除非你加长度限制）
func (q *BlockingQueue[T]) Push(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		zap.L().Warn("queue has been closed, ignore push")
		return ErrQueueClosed
	}

	q.data = append(q.data, v)
	q.cond.Signal() // 唤醒一个等待的 Pop
	zap.L().Debug("push", zap.Any("value", v))
	return nil
}

// Pop 从队列取出元素，如果为空则阻塞，直到有数据或队列被清空/关闭
// 返回值: value, ok
//
//	ok == true: 正常取出数据
//	ok == false: 队列已关闭或被清空后无新数据
func (q *BlockingQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 等待直到有数据或被关闭（但 Push 后仍可能有数据）
	for len(q.data) == 0 && !q.closed {
		q.cond.Wait()
	}

	if len(q.data) > 0 {
		v := q.data[0]
		q.data = q.data[1:]
		zap.L().Debug("pop", zap.Any("value", v))
		return v, true
	}

	var zero T
	return zero, false
}

// Clear 清空当前所有缓存的数据
func (q *BlockingQueue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	zap.L().Info("queue cleared", zap.Int("totalMessages", len(q.data)))
	q.data = q.data[:0] // 清空 slice
}

// Close 关闭队列，唤醒所有等待的 Pop，使其返回 (zero, false)
func (q *BlockingQueue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.data = q.data[:0]
	q.cond.Broadcast() // 唤醒所有等待者
	zap.L().Info("queue closed")
}
