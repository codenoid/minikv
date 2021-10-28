package minikv

import (
	"errors"
	"sync"
	"time"
)

const (
	// For use with functions that take an expiration time.
	NoExpiration time.Duration = -1
	// For use with functions that take an expiration time. Equivalent to
	// passing in the same expiration duration as was given to New() or
	// NewFrom() when the cache was created (e.g. 5 minutes.)
	DefaultExpiration time.Duration = 0
)

type Item struct {
	Key        string
	Object     interface{}
	Expiration int64
}

func (item Item) Expired() bool {
	now := time.Now().UnixNano()
	return now > item.Expiration
}

type KV struct {
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
	items             sync.Map
	mu                sync.RWMutex
	onDeleted         func(string, interface{})
	onEvicted         func(string, interface{})
}

func New(defaultExpiration, cleanupInterval time.Duration) *KV {

	new := KV{
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
	}

	go new.runJanitor()

	return &new
}

func (kv *KV) OnEvicted(f func(string, interface{})) {
	kv.mu.Lock()
	kv.onEvicted = f
	kv.mu.Unlock()
}

func (kv *KV) OnDeleted(f func(string, interface{})) {
	kv.mu.Lock()
	kv.onDeleted = f
	kv.mu.Unlock()
}

func (kv *KV) Set(key string, value interface{}, exp time.Duration) {

	expiredAt := int64(0)

	switch exp {
	case DefaultExpiration:
		expiredAt = time.Now().Add(kv.defaultExpiration).UnixNano()
	case NoExpiration:
		// explicitly
		expiredAt = 0
	default:
		expiredAt = time.Now().Add(exp).UnixNano()
	}

	item := Item{
		Key:        key,
		Expiration: expiredAt,
		Object:     value,
	}
	kv.items.Store(key, item)
}

func (kv *KV) Update(key string, value interface{}) error {

	item := Item{}
	if obj, ok := kv.items.Load(key); ok {
		item = obj.(Item)
	} else {
		return errors.New("object doesn't exist")
	}

	item.Object = value
	kv.items.Store(key, item)

	return nil
}

func (kv *KV) Get(key string) (interface{}, bool) {

	now := time.Now().UnixNano()

	if obj, ok := kv.items.Load(key); ok {
		val := obj.(Item)
		if val.Expiration > now {
			return val.Object, true
		}
	}

	return nil, false
}

func (kv *KV) Delete(key string) error {

	if obj, ok := kv.items.Load(key); ok {
		val := obj.(Item)
		if kv.onDeleted != nil {
			go kv.onDeleted(key, val.Object)
		}
		kv.items.Delete(key)
	} else {
		return errors.New("key args not exist")
	}

	return nil
}

func (kv *KV) List() map[string]Item {

	now := time.Now().UnixNano()
	m := make(map[string]Item)

	kv.items.Range(func(key interface{}, value interface{}) bool {
		item := value.(Item)
		if item.Expiration > now {
			m[item.Key] = item
		}
		return true
	})

	return m
}

func (kv *KV) ListAll() map[string]Item {
	m := make(map[string]Item)

	kv.items.Range(func(key interface{}, value interface{}) bool {
		item := value.(Item)
		m[item.Key] = item
		return true
	})

	return m
}

func (kv *KV) IsExist(key string) bool {
	now := time.Now().UnixNano()

	if obj, ok := kv.items.Load(key); ok {
		val := obj.(Item)
		if val.Expiration > now {
			return true
		}
	}

	return false
}

func (kv *KV) IsExpired(key string) (bool, error) {
	now := time.Now().UnixNano()

	if obj, ok := kv.items.Load(key); ok {
		val := obj.(Item)
		if val.Expiration > now {
			return false, nil
		}
		return true, nil
	}

	return false, errors.New("requested key are not exist")
}

func (kv *KV) ItemCount() int {
	total := 0

	now := time.Now().UnixNano()
	kv.items.Range(func(key interface{}, value interface{}) bool {
		item := value.(Item)
		if item.Expiration > now {
			total++
		}
		return true
	})

	return total
}

func (kv *KV) ItemCountAll() int {
	total := 0

	kv.items.Range(func(key interface{}, value interface{}) bool {
		total++
		return true
	})

	return total
}

func (kv *KV) DeleteExpired() {

	now := time.Now().UnixNano()

	kv.items.Range(func(key interface{}, value interface{}) bool {
		item := value.(Item)
		if now > item.Expiration {
			if kv.onEvicted != nil {
				go kv.onEvicted(item.Key, item.Object)
			}
			kv.items.Delete(key)
		}
		return true
	})
}

func (kv *KV) Flush() {
	kv.items.Range(func(key interface{}, value interface{}) bool {
		kv.items.Delete(key)
		return true
	})
}

func (kv *KV) runJanitor() {
	if kv.cleanupInterval != NoExpiration {
		ticker := time.NewTicker(kv.cleanupInterval)
		for range ticker.C {
			kv.DeleteExpired()
		}
	}
}
