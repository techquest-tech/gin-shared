//go:build !ram

package cache_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
)

type CDemo struct {
	Value string
}

func TestRedisCache(t *testing.T) {
	core.BeforeBootup("")

	rr := cache.NewCacheProvider[string](10 * time.Minute)

	value := "Hello Redis"
	k := "demo"

	rr.Set(k, value)

	cachedValue, ok := rr.Get(k)
	assert.True(t, ok)
	assert.Equal(t, value, cachedValue)

	r2 := cache.NewCacheProvider[*CDemo](10 * time.Minute)

	r2.Set(k, &CDemo{Value: value})

	v2, ok := r2.Get(k)
	assert.True(t, ok)
	assert.Equal(t, value, v2.Value)
}
