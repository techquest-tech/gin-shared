package locker_test

import (
	"context"
	"log"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	_ "github.com/techquest-tech/gin-shared/pkg/cache"
	"github.com/techquest-tech/gin-shared/pkg/core"
	"github.com/techquest-tech/gin-shared/pkg/locker"
	"github.com/thanhpk/randstr"
)

func init() {
	core.BeforeBootup("locker")
}

func TestRedisLocker(t *testing.T) {
	err := core.GetContainer().Invoke(func(locker locker.Locker) {
		resourceName := "test"
		g := sync.WaitGroup{}
		g.Add(2)
		// ts := 10 * time.Second
		go func() {
			defer g.Done()
			ctx := context.Background()
			release, err := locker.Lock(ctx, resourceName)
			assert.Nil(t, err)
			log.Print("get lock done for go routine 1")
			defer release(ctx)
			time.Sleep(time.Second * 5)
		}()

		time.Sleep(time.Second)
		go func() {
			defer g.Done()
			ctx := context.Background()
			release, err := locker.Lock(ctx, resourceName)
			if err == nil {
				release(ctx)
			}
			assert.NotNil(t, err)
			if err != nil {
				log.Print("get lock failed for go routine 2, " + err.Error())
			}

		}()

		g.Wait()
	})
	assert.Nil(t, err)
}
func TestRedisLockerAtSameTime(t *testing.T) {
	timeout := 2 * time.Second
	err := core.GetContainer().Invoke(func(locker locker.Locker) {
		resourceName := "same_time"
		// g := sync.WaitGroup{}
		// g.Add(2)
		r1 := make(chan bool)
		r2 := make(chan bool)
		fn := func(r chan bool) {
			k := randstr.Hex(3)
			ctx := context.Background()
			release, err := locker.Lock(ctx, resourceName)
			if err == nil {
				defer release(ctx)
			}
			if err == nil {
				log.Print("get lock done for " + k)
				r <- true
			} else {
				log.Print("get lock failed for " + k)
				r <- false
			}
			time.Sleep(timeout)
		}
		go fn(r1)
		go fn(r2)
		var rr1 bool
		var rr2 bool
		for i := 0; i < 2; i++ {
			select {
			case rr1 = <-r1:
			case rr2 = <-r2:
			}
		}
		time.Sleep(timeout + time.Second)
		assert.False(t, rr1 && rr2)
		assert.False(t, !rr1 && !rr2)
		assert.True(t, rr1 || rr2)
	})
	assert.Nil(t, err)
}
func TestRedisLocker2(t *testing.T) {
	err := core.GetContainer().Invoke(func(locker locker.Locker) {
		resourceName := "test"
		g := sync.WaitGroup{}
		g.Add(2)
		// ts := 10 * time.Second
		go func() {
			defer g.Done()
			ctx := context.Background()
			release, err := locker.Lock(ctx, resourceName)
			assert.Nil(t, err)
			log.Print("get lock done for go routine 1")
			defer release(ctx)
			time.Sleep(time.Second * 2)
		}()

		// locker should be released
		time.Sleep(3 * time.Second)
		go func() {
			defer g.Done()
			ctx := context.Background()
			release, err := locker.Lock(ctx, resourceName)
			assert.Nil(t, err)
			log.Print("get lock done for go routine 2, pre locker should be released")
			defer release(ctx)
		}()

		g.Wait()
	})
	assert.Nil(t, err)
}

func TestLockerFailed(t *testing.T) {
	err := core.GetContainer().Invoke(func(locker locker.Locker) {
		resourceName := "test"
		ctx := context.Background()
		fn := func(index int) {
			release, err := locker.LockWithtimeout(ctx, resourceName, 5*time.Second)
			// assert.NotNil(t, err)
			if err != nil {
				log.Print("get lock failed for " + strconv.Itoa(index) + ", " + err.Error())
			} else {
				time.Sleep(2 * time.Second)
				release(ctx)
				log.Print("release lock done for " + strconv.Itoa(index))
			}
		}

		g := sync.WaitGroup{}
		g.Add(10)
		for i := 0; i < 10; i++ {
			go fn(i)
			time.Sleep(1 * time.Second)
			g.Done()
		}
		g.Wait()
	})
	assert.Nil(t, err)
}
