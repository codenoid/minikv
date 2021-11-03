package minikv

import (
	"testing"
	"time"
)

func TestExistence(t *testing.T) {
	kv := New(5*time.Second, NoExpiration)

	kv.Set("name", "mike", time.Second)
	if kv.IsExist("name") != true {
		t.Error("name should exist (called before expired / deletion)")
	}

	time.Sleep(1500 * time.Millisecond)

	if kv.IsExist("name") == true {
		t.Error("Found key that should be expired: name")
	}
}

func TestExistence_NoExpiration(t *testing.T) {
	kv := New(5*time.Second, 1 * time.Second)

	kv.Set("name", "mike", NoExpiration)
	if kv.IsExist("name") != true {
		t.Error("name should exist (called before expired / deletion)")
	}

	time.Sleep(1500 * time.Millisecond)

	if kv.IsExist("name") == false {
		t.Error("Lost key that should be present: name")
	}

	kv.Delete("name")

	if kv.IsExist("name") == true {
		t.Error("Found key that should be removed: name")
	}
}

func TestJanitor(t *testing.T) {

	onEvictCalled := false

	kv := New(200*time.Millisecond, 500*time.Millisecond)
	kv.OnEvicted(func(key string, value interface{}) {
		onEvictCalled = true
	})

	kv.Set("name", "mike", DefaultExpiration)

	time.Sleep(100 * time.Millisecond)

	if kv.ItemCountAll() != 1 {
		t.Error("key should be still exist")
	}

	// should already purged, but just making sure
	time.Sleep(500 * time.Millisecond)

	if kv.ItemCount() != 0 {
		t.Error(".ItemCountAll() should be empty by global janitor")
	}

	if !onEvictCalled {
		t.Error("OnEvicted should be called")
	}
}
