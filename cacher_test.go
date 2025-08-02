package cacher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacher_SetAndGet(t *testing.T) {
	cfg := Config{
		Capacity:         10,
		ClearingInterval: 100 * time.Millisecond,
		EvictionPolicy:   LRU,
	}
	cache := New(cfg)

	key, value := "test_key", "test_value"
	cache.Set(key, value, 5*time.Second)

	got, err := cache.Get(key)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestCacher_GetExpired(t *testing.T) {
	cfg := Config{
		Capacity:         10,
		ClearingInterval: 10 * time.Millisecond,
		EvictionPolicy:   LRU,
	}
	cache := New(cfg)

	key, value := "exp_key", "exp_value"
	cache.Set(key, value, 20*time.Millisecond)

	time.Sleep(30 * time.Millisecond)

	_, err := cache.Get(key)
	assert.Error(t, err)
}

func TestCacher_GetAll(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)

	all := cache.GetAll()
	assert.Len(t, all, 2)
	assert.Contains(t, all, "v1")
	assert.Contains(t, all, "v2")
}

func TestCacher_Clear(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Clear()

	_, err := cache.Get("k1")
	assert.Error(t, err)
}

func TestCacher_Delete(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	key, value := "del_key", "del_value"
	cache.Set(key, value, 5*time.Second)

	err := cache.Delete(key)
	require.NoError(t, err)

	_, err = cache.Get(key)
	assert.Error(t, err)
}

func TestCacher_Capacity(t *testing.T) {
	cfg := Config{Capacity: 2, EvictionPolicy: LRU}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)
	cache.Set("k3", "v3", 5*time.Second) // должен вытеснить k1 (LRU)

	_, err := cache.Get("k1")
	assert.Error(t, err)
	_, err = cache.Get("k2")
	assert.NoError(t, err)
	_, err = cache.Get("k3")
	assert.NoError(t, err)
}

func TestCacher_LRU(t *testing.T) {
	cfg := Config{Capacity: 2, EvictionPolicy: LRU}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)
	cache.Get("k1")                      // используем k1 -> становится MRU
	cache.Set("k3", "v3", 5*time.Second) // k2 должен быть вытеснен

	_, err := cache.Get("k2")
	assert.Error(t, err)
	_, err = cache.Get("k1")
	assert.NoError(t, err)
	_, err = cache.Get("k3")
	assert.NoError(t, err)
}

func TestCacher_MRU(t *testing.T) {
	cfg := Config{Capacity: 2, EvictionPolicy: MRU}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)
	cache.Set("k3", "v3", 5*time.Second) // MRU: k2 — самый свежий, его и вытесняем

	_, err := cache.Get("k2")
	assert.Error(t, err)
	_, err = cache.Get("k1")
	assert.NoError(t, err)
	_, err = cache.Get("k3")
	assert.NoError(t, err)
}

func TestCacher_LFU(t *testing.T) {
	cfg := Config{Capacity: 2, EvictionPolicy: LFU}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second) // counter: 1
	cache.Set("k2", "v2", 5*time.Second) // counter: 1
	cache.Get("k1")                      // counter: 2
	cache.Get("k1")                      // counter: 3
	cache.Set("k3", "v3", 5*time.Second) // k2 (counter=1) вытесняется

	_, err := cache.Get("k2")
	assert.Error(t, err)
	_, err = cache.Get("k1")
	assert.NoError(t, err)
	_, err = cache.Get("k3")
	assert.NoError(t, err)
}

func TestCacher_RANDOM(t *testing.T) {
	cfg := Config{Capacity: 2, EvictionPolicy: RANDOM}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)
	cache.Set("k3", "v3", 5*time.Second) // одно из двух: k1 или k2 будет вытеснено

	// Проверим, что хотя бы один остался
	_, err1 := cache.Get("k1")
	_, err2 := cache.Get("k2")

	// Один должен быть, другой — нет
	assert.True(t, (err1 == nil) != (err2 == nil)) // XOR
	_, err := cache.Get("k3")
	assert.NoError(t, err)
}

func TestCacher_TTLUpdate(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	key := "ttl_key"
	cache.Set(key, "value", 5*time.Second)

	err := cache.SetTTL(key, 10*time.Second)
	require.NoError(t, err)

	// Проверим, что TTL изменился (косвенно)
	time.Sleep(6 * time.Second)
	_, err = cache.Get(key)
	assert.NoError(t, err) // не должен быть удалён
}

func TestCacher_GetTTL(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	key := "ttl_key"
	cache.Set(key, "value", 5*time.Second)

	ttl, err := cache.GetTTL(key)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, ttl)
}

func TestCacher_GetCounter(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	key := "counter_key"
	cache.Set(key, "value", 5*time.Second)
	cache.Get(key) // +1
	cache.Get(key) // +1

	counter, err := cache.GetCounter(key)
	require.NoError(t, err)
	assert.Equal(t, 3, counter) // 1 (Set) + 2 (Get)
}

func TestCacher_Keys(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)

	keys, err := cache.Keys()
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "k1")
	assert.Contains(t, keys, "k2")
}

func TestCacher_Stats(t *testing.T) {
	cfg := Config{
		Capacity:         100,
		ClearingInterval: time.Second,
		EvictionPolicy:   LRU,
	}
	cache := New(cfg)

	cache.Set("k1", "v1", 5*time.Second)
	cache.Set("k2", "v2", 5*time.Second)

	stats := cache.Stats()

	assert.Contains(t, stats, "Eviction Policy: LRU")
	assert.Contains(t, stats, "Capacity: 100")
	assert.Contains(t, stats, "Clearing Interval: 1s")
	assert.Contains(t, stats, "Items: 2")
	assert.Contains(t, stats, "Occupancy: 2.00%")
	assert.Contains(t, stats, "Key: k1 Value: v1")
	assert.Contains(t, stats, "Key: k2 Value: v2")
}

func TestCacher_SetCapacity(t *testing.T) {
	cfg := Config{Capacity: 10}
	cache := New(cfg)

	err := cache.SetCapacity(5)
	require.NoError(t, err)
	assert.Equal(t, 5, cache.GetCapacity())

	err = cache.SetCapacity(-1)
	assert.Error(t, err)
}

func TestCacher_SetEvictionPolicy(t *testing.T) {
	cfg := Config{EvictionPolicy: LRU}
	cache := New(cfg)

	err := cache.SetEvictionPolicy(MRU)
	require.NoError(t, err)
	assert.Equal(t, "MRU", cache.GetEvictionPolicy())

	err = cache.SetEvictionPolicy(10)
	assert.Error(t, err)
}

func TestCacher_Close(t *testing.T) {
	cfg := Config{ClearingInterval: 100 * time.Millisecond}
	cache := New(cfg)

	cache.Close()
	// Проверим, что очистка остановилась
	time.Sleep(200 * time.Millisecond)
	// Нет паники — хорошо
}
