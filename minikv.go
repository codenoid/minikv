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

type meta struct {
	Key        string
	Expiration int64
}

type updateMeta struct {
	meta
	PriorExpiration int64
}

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

	metaAdd    chan meta
	metaUpdate chan updateMeta
	metaDelete chan meta

	exp map[int64]map[string]*struct{}
}

func New(defaultExpiration, cleanupInterval time.Duration) *KV {

	new := KV{
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
		//TODO: configure capacities
		metaAdd:    make(chan meta, 10),
		metaUpdate: make(chan updateMeta, 10),
		metaDelete: make(chan meta, 10),
		exp:        make(map[int64]map[string]*struct{}),
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

	if prior, found := kv.items.Load(key); !found {
		kv.items.Store(key, item)

		kv.metaAdd <- meta{
			Key:        item.Key,
			Expiration: item.Expiration,
		}
	} else {
		priorItem := prior.(Item)

		kv.items.Store(key, item)

		if item.Expiration != priorItem.Expiration {
			kv.metaUpdate <- updateMeta{
				meta: meta{
					Key:        key,
					Expiration: expiredAt,
				},
				PriorExpiration: priorItem.Expiration,
			}
		}
	}
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

	kv.metaAdd <- meta{
		Key:        item.Key,
		Expiration: item.Expiration,
	}

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
	itm, err := kv.deleteInner(key)
	if err != nil {
		return err
	}
	kv.metaDelete <- meta{
		Key:        itm.Key,
		Expiration: itm.Expiration,
	}
	return nil
}

func (kv *KV) deleteInner(key string) (val Item, err error) {
	if obj, ok := kv.items.Load(key); ok {
		val := obj.(Item)
		if kv.onDeleted != nil {
			go kv.onDeleted(key, val.Object)
		}
		kv.items.Delete(key)
	} else {
		return val, errors.New("key args not exist")
	}
	return val, err
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
	var expired int64

	for expiration, keys := range kv.exp {
		if expiration < now && len(keys) > 0 {
			for k, _ := range keys {
				// dont send metadata updates for janitor
				itm, err := kv.deleteInner(k)

				if err == nil && kv.onEvicted != nil {
					go kv.onEvicted(itm.Key, itm.Expiration)
				}
			}

			expired = expiration
			break //TODO: only going to process first non-empty item in map on each pass, be nice to enforce ordering
		}
	}

	delete(kv.exp, expired)
}

func (kv *KV) Flush() {
	kv.items.Range(func(key interface{}, value interface{}) bool {
		kv.items.Delete(key)
		return true
	})
}

func (kv *KV) runJanitor() {
	for {
		select {
		case lm := <-kv.metaAdd:
			if curr, found := kv.exp[lm.Expiration]; found {
				curr[lm.Key] = &struct{}{}
				kv.exp[lm.Expiration] = curr
			} else {
				kv.exp[lm.Expiration] = map[string]*struct{}{lm.Key: {}}
			}
		case mu := <-kv.metaUpdate:
			if curr, found := kv.exp[mu.PriorExpiration]; found {
				delete(curr, mu.Key)
				kv.exp[mu.PriorExpiration] = curr
			}

			if curr, found := kv.exp[mu.Expiration]; found {
				curr[mu.Key] = &struct{}{}
				kv.exp[mu.Expiration] = curr
			} else {
				kv.exp[mu.Expiration] = map[string]*struct{}{mu.Key: {}}
			}
		case md := <-kv.metaDelete:
			if curr, found := kv.exp[md.Expiration]; found {
				delete(curr, md.Key)
				kv.exp[md.Expiration] = curr
			}
		default:
			kv.DeleteExpired()
		}
	}
}
