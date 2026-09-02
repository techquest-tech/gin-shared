package core

import (
	"crypto/md5"
	"encoding/hex"
	"sync"
	"time"
)

// Deduper is a small in-memory duplicate detector keyed by the md5 of the
// serialized payload. It is shared by the chan-based adaptor and the
// redis-streaming adaptor so both implementations apply the same dedup policy.
//
// A message is considered a duplicate when the same hash was seen within
// window. Expired entries are lazily purged on each Seen call.
type Deduper struct {
	window time.Duration
	locker sync.Mutex
	items  map[string]time.Time
}

// NewDeduper creates a deduper with the given window. A non-positive window
// disables dedup (Seen always returns false).
func NewDeduper(window time.Duration) *Deduper {
	return &Deduper{
		window: window,
		items:  make(map[string]time.Time),
	}
}

// Seen registers raw and reports whether it is a duplicate within the window.
// The first occurrence returns false (not a duplicate) and is remembered;
// following identical payloads within the window return true.
func (d *Deduper) Seen(raw []byte) bool {
	if d == nil || d.window <= 0 {
		return false
	}
	sum := md5.Sum(raw)
	hash := hex.EncodeToString(sum[:])
	now := time.Now()

	d.locker.Lock()
	defer d.locker.Unlock()

	for k, t := range d.items {
		if now.Sub(t) > d.window {
			delete(d.items, k)
		}
	}
	if t, ok := d.items[hash]; ok && now.Sub(t) <= d.window {
		return true
	}
	d.items[hash] = now
	return false
}
