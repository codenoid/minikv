# MiniKV

![Go Test](https://github.com/codenoid/minikv/actions/workflows/go.yml/badge.svg)

Rewriten from [patrickmn/go-cache](https://github.com/patrickmn/go-cache) with sync.Map and expected behavior (drop-in replacement).

minikv is an in-memory key:value store/cache that is suitable for applications running on a single machine. Its major advantage is that, being essentially a thread-safe map[string]interface{} with expiration times, it doesn't need to serialize or transmit its contents over the network.

Any object can be stored, for a given duration or forever, and the cache can be safely used by multiple goroutines.

## Installation

`go get github.com/codenoid/minikv`

## Usage

For detailed API's, [go here](https://pkg.go.dev/github.com/codenoid/minikv)

```go
package main

import (
	"fmt"
	"github.com/codenoid/minikv"
	"time"
)

func main() {
    kv := minikv.New(5*time.Minute, 10*time.Minute)

    // Listen to what has been removed or expired
    kv.OnEvicted(func(key string, value interface{}) {
        fmt.Println(key, "has been evicted")
    })

    // Set the value of the key "foo" to "bar", with the default expiration time
    // which is 5*time.Minute
    kv.Set("foo", "bar", cache.DefaultExpiration)

    // Get the string associated with the key "foo" from the cache
    foo, found := c.Get("foo")
    if found {
        fmt.Println(foo)
    }
}
```
