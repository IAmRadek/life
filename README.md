# Life

A Go library for managing the lifecycle of an application with graceful shutdown capabilities.

## Overview

The Life library provides a simple mechanism for managing the lifetime of an application. It helps you handle application startup, running, and shutdown phases with proper resource cleanup. Key features include:

- Distinct application lifecycle phases (starting, running, teardown)
- Context-based lifecycle management
- Graceful shutdown with customizable timeout
- Flexible callback registration for cleanup operations
- Signal handling for clean application termination
- Synchronous and asynchronous shutdown callbacks
- Error handling during shutdown

## Installation

```bash
go get github.com/IAmRadek/life
```

## Usage

### Basic Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IAmRadek/life"
)

func main() {
	// Create a new Life instance with signal handling for graceful shutdown
	l := life.New(
		life.WithSignal(syscall.SIGINT, syscall.SIGTERM),
		life.WithTeardownTimeout(5*time.Second),
	)

	// Register cleanup functions
	l.OnExit(func() {
		fmt.Println("Cleaning up resources...")
		time.Sleep(1 * time.Second)
		fmt.Println("Cleanup complete")
	})

	// Start your application
	fmt.Println("Application starting...")

	// Run the application (blocks until Die() is called)
	if err := l.Run(); err != nil {
		fmt.Printf("Application exited with error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Application exited gracefully")
}
```

### Advanced Example with Context

```go
package main

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/IAmRadek/life"
)

func main() {
	// Create a new Life instance with options
	l := life.New(
		life.WithSignal(syscall.SIGINT, syscall.SIGTERM),
		life.WithTeardownTimeout(10*time.Second),
		life.WithExitErrorCallback(func(err error) {
			fmt.Printf("Error during shutdown: %v\n", err)
		}),
	)

	// Use the starting context for initialization
	startingCtx := l.StartingContext()
	// Initialize resources with the starting context

	// Register cleanup with context awareness
	l.OnExitWithContext(func(ctx context.Context) {
		fmt.Println("Starting cleanup...")

		select {
		case <-time.After(2 * time.Second):
			fmt.Println("Cleanup completed successfully")
		case <-ctx.Done():
			fmt.Println("Cleanup was interrupted by timeout")
		}
	})

	// Register cleanup that might return an error
	l.OnExitWithContextError(func(ctx context.Context) error {
		fmt.Println("Closing database connection...")
		time.Sleep(1 * time.Second)
		return nil
	})

	// Register an async cleanup task
	l.OnExit(func() {
		fmt.Println("Performing async cleanup...")
		time.Sleep(3 * time.Second)
		fmt.Println("Async cleanup complete")
	}, life.Async)

	// Start your application
	fmt.Println("Application starting...")

	// Get the running context to use in your application
	runningCtx := l.Context()

	// Start a worker that respects the application lifecycle
	go func() {
		for {
			select {
			case <-runningCtx.Done():
				fmt.Println("Worker shutting down...")
				return
			case <-time.After(1 * time.Second):
				fmt.Println("Worker doing work...")
			}
		}
	}()

	// Run the application (blocks until Die() is called)
	if err := l.Run(); err != nil {
		fmt.Printf("Application exited with error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Application exited gracefully")
}
```

## License

[MIT License](LICENSE)
