package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/scottdavis/dsgo/pkg/agents/memory"
)

// Simple Redis example to demonstrate basic Redis storage functionality

// RunSimpleRedisExample demonstrates basic Redis operations
func RunSimpleRedisExample() {
	// Parse command line flags
	redisAddr := flag.String("redis-addr", "localhost:6379", "Redis server address")
	redisPassword := flag.String("redis-password", "", "Redis password")
	redisDB := flag.Int("redis-db", 0, "Redis database number")
	flag.Parse()

	// Setup simple logging
	fmt.Println("Starting simple Redis example...")

	// Create a Redis store
	store, err := memory.NewRedisStore(*redisAddr, *redisPassword, *redisDB)
	if err != nil {
		fmt.Printf("Error connecting to Redis: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Clear any existing data
	if err := store.Clear(); err != nil {
		fmt.Printf("Error clearing Redis store: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Redis store cleared")

	// Store a simple key-value pair
	err = store.Store("greeting", "Hello from DSP-GO Redis example!")
	if err != nil {
		fmt.Printf("Error storing value: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Stored greeting")

	// Store a numeric value
	err = store.Store("counter", 42)
	if err != nil {
		fmt.Printf("Error storing counter: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Stored counter")

	// Store a value with TTL (time-to-live)
	ctx := context.Background()
	err = store.StoreWithTTL(ctx, "temporary", "This value will expire soon", 5*time.Second)
	if err != nil {
		fmt.Printf("Error storing temporary value: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Stored temporary value with 5 second TTL")

	// Retrieve values
	greeting, err := store.Retrieve("greeting")
	if err != nil {
		fmt.Printf("Error retrieving greeting: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Retrieved greeting: %s\n", greeting)

	counter, err := store.Retrieve("counter")
	if err != nil {
		fmt.Printf("Error retrieving counter: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Retrieved counter: %v (type: %T)\n", counter, counter)

	// List all keys
	keys, err := store.List()
	if err != nil {
		fmt.Printf("Error listing keys: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d keys in Redis store:\n", len(keys))
	for _, key := range keys {
		fmt.Printf("  - %s\n", key)
	}

	// Wait for TTL to expire
	fmt.Println("\nWaiting 6 seconds for the temporary value to expire...")
	time.Sleep(6 * time.Second)

	// Try to retrieve the expired value
	_, err = store.Retrieve("temporary")
	if err != nil {
		fmt.Println("As expected, the temporary value has expired")
	} else {
		fmt.Println("Unexpected: the temporary value is still available")
	}

	fmt.Println("\nSimple Redis example completed successfully")
}
