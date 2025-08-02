🚀 Go Cacher
A lightweight, thread-safe, in-memory cache for Go with support for multiple eviction policies, TTL (time-to-live), and automatic cleanup.

Perfect for caching database results, API responses, or any frequently accessed data.

✨ Features
✅ Multiple eviction policies:
- LRU – Least Recently Used
- MRU – Most Recently Used
- LFU – Least Frequently Used
- RANDOM – Random eviction
⏳ TTL Support: Set expiration time for each cached item
🧹 Auto-clearing: Background goroutine removes expired items
🔐 Thread-safe: Uses sync.RWMutex for concurrent access
📏 Configurable capacity: Limit cache size
📊 Stats & Diagnostics: View cache stats, occupancy, and contents
🔄 Dynamic reconfiguration: Change capacity and policy at runtime
🛑 Graceful shutdown: Stop background cleanup with Close()

# 🛠️ Installation

go get github.com/danRulev/cacher

# 🚀 Usage

package main

import (
    "fmt"
    "time"
    "github.com/your-username/cacher"
)

func main() {
    // Configure the cache
    cfg := cacher.Config{
        Capacity:         100,
        ClearingInterval: 10 * time.Second,
        EvictionPolicy:   cacher.LRU,
    }

    // Create a new cache
    cache := cacher.New(cfg)

    // Set a value with 5-second TTL
    cache.Set("key1", "value1", 5*time.Second)

    // Get a value
    if val, err := cache.Get("key1"); err == nil {
        fmt.Println("Found:", val) // Output: Found: value1
    }

    // Wait for expiration
    time.Sleep(6 * time.Second)

    // Try to get expired value
    if _, err := cache.Get("key1"); err != nil {
        fmt.Println("Cache expired:", err)
    }

    // Close the cache (stop background cleanup)
    cache.Close()
//}

# ⚙️ Configuration
// type Config struct {
    Capacity         int           // Max number of items (0 = unlimited)
    ClearingInterval time.Duration // How often to check for expired items
    EvictionPolicy   int           // LRU, MRU, LFU, or RANDOM
}





