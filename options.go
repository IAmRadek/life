package life

import (
	"fmt"
	"os"
	"os/signal"
	"time"
)

type opt func(*Life)

// WithExitErrorCallback sets the callback that will be called when an error occurs during the teardown phase.
func WithExitErrorCallback(callback func(error)) opt {
	return func(l *Life) {
		l.errorHandler = callback
	}
}

// WithTeardownTimeout sets the timeout for the teardown phase.
func WithTeardownTimeout(timeout time.Duration) opt {
	return func(l *Life) {
		l.teardownTimeout = timeout
	}
}

// WithStartingTimeout sets the timeout for the starting phase.
func WithStartingTimeout(timeout time.Duration) opt {
	return func(l *Life) {
		l.startingTimeout = timeout
	}
}

var ErrSignaled = fmt.Errorf("received signal")

// WithSignal calls Life.Die() when the specified signal is received.
func WithSignal(s1 os.Signal, sMany ...os.Signal) opt {
	return func(l *Life) {
		notify := make(chan os.Signal, 1)
		signal.Notify(notify, append([]os.Signal{s1}, sMany...)...)

		go func() {
			sig := <-notify
			l.Die(fmt.Errorf("%w: %q", ErrSignaled, sig))
		}()
	}
}
