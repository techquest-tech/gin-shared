package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/techquest-tech/gin-shared/pkg/core"
	"go.uber.org/zap"
)

type HashEx[T any] struct {
	data Hash
}

func (h *HashEx[T]) Existed(ctx context.Context, key string) (bool, error) {
	return h.data.Existed(ctx, key)
}
func (h *HashEx[T]) SetTTL(ctx context.Context, key string, ttl time.Duration) {
	h.data.SetTTL(ctx, key, ttl)
}
func (h *HashEx[T]) GetValues(ctx context.Context, key string, fields ...string) ([]*T, error) {
	result := make([]*T, len(fields))

	mresult, err := h.data.GetValues(ctx, key, fields...)
	if err != nil {
		return nil, err
	}
	for index, item := range mresult {
		if item == nil {
			continue
		}
		// check if can convert to T
		switch cache := item.(type) {
		case T:
			result[index] = &cache
		case *T:
			result[index] = cache
		case []byte:
			var t T
			err := json.Unmarshal(cache, &t)
			if err != nil {
				return nil, err
			}
			result[index] = &t
		case string:
			var t T
			err := json.Unmarshal([]byte(cache), &t)
			if err != nil {
				return nil, err
			}
			result[index] = &t

		default:
			zap.L().Error("unknown data type", zap.Error(err), zap.Any("raw", item))
			return nil, err
		}
	}
	return result, nil
}
func (h *HashEx[T]) GetValue(ctx context.Context, key string, field string) (*T, error) {
	values, err := h.GetValues(ctx, key, field)
	if err != nil {
		return nil, err
	}
	return values[0], nil
}

func (h *HashEx[T]) SetValues(ctx context.Context, key string, values map[string]T) error {
	data := make(map[string]any)
	for k, v := range values {
		// check if v is struct
		if core.IsStructOrPtrToStruct(v) {
			vv, err := json.Marshal(v)
			if err != nil {
				return err
			}
			data[k] = vv
		} else {
			data[k] = v
		}

	}
	return h.data.SetValues(ctx, key, data)
}

func NewHashEx[T any]() *HashEx[T] {
	var result *HashEx[T]
	core.GetContainer().Invoke(func(hash Hash) {
		result = &HashEx[T]{
			data: hash,
		}
	})
	return result
}
