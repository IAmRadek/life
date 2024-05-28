/*
Package lifecycle implements simple mechanism for managing a lifetime of an application.
It provides a way to register a functions that will be called when the application is about to exit.
It distinguishes between starting, running and teardown phases.

The application is considered to be starting before calling Life.Run().
You can use Life.StartingContext() to get a context that can be used to control the startup phase.
Starting context is cancelled when the startup phase is over.

The application is considered to be running after calling Life.Run() and before calling Life.Die().
You can use Life.Context() to get a context that can be used to control the running phase.
It is cancelled when the application is about to exit.

The application is considered to be tearing down after calling Life.Die().
You can use Life.TeardownContext() to get a context that can be used to control the teardown phase.
It is cancelled when the teardown timeout is reached.
*/
package life

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type Life struct {
	phase atomic.Int32
	die   chan struct{}

	runningCtx    context.Context
	runningCancel context.CancelCauseFunc

	startingCtx     context.Context
	startingCancel  context.CancelFunc
	startingTimeout time.Duration

	teardownCtx     context.Context
	teardownCancel  context.CancelFunc
	teardownTimeout time.Duration

	callbacks []*callback

	errorHandler func(error)
}

// New creates a new instance of Life.
func New(opts ...opt) *Life {
	runningCtx, runningCancel := context.WithCancelCause(context.Background())
	teardownCtx, teardownCancel := context.WithCancel(context.Background())
	l := &Life{
		die:       make(chan struct{}),
		callbacks: make([]*callback, 0),

		runningCtx:    runningCtx,
		runningCancel: runningCancel,

		teardownCtx:    teardownCtx,
		teardownCancel: teardownCancel,
	}
	for _, opt := range opts {
		opt(l)
	}

	l.start()

	return l
}

func (l *Life) start() {
	l.phase.Store(int32(phaseStarting))

	ctx, cancel := context.WithCancel(context.Background())
	if l.startingTimeout > 0 {
		ctx, _ = context.WithTimeout(ctx, l.startingTimeout)
	}

	l.startingCtx = ctx
	l.startingCancel = cancel
}

// StartingContext returns a context that will be cancelled after the starting timeout.
// It can be used to control the startup of the application. It will be cancelled after calling Life.Run().
func (l *Life) StartingContext() context.Context {
	return l.startingCtx
}

// Context returns a context that will be cancelled when the application is about to exit.
// It can be used to control the lifetime of the application. It will be cancelled after calling Life.Die().
func (l *Life) Context() context.Context {
	return l.runningCtx
}

// OnExit registers a callback that will be called when the application is about to exit.
// Has no effect after calling Life.Run().
// Use life.Async option to execute the callback in a separate goroutine.
// life.PanicOnError has no effect on this function.
func (l *Life) OnExit(callback func(), exitOpts ...exitCallbackOpt) {
	l.addCallback(func(ctx context.Context) error {
		return callbackWithContext(ctx, func() error {
			callback()
			return nil
		})
	}, exitOpts...)
}

// OnExitWithError registers a callback that will be called when the application is about to exit.
// Has no effect after calling Life.Run().
// The callback can return an error that will be passed to the error handler.
// Use life.Async option to execute the callback in a separate goroutine.
// Use life.PanicOnError to panic with the error returned by the callback.
func (l *Life) OnExitWithError(callback func() error, exitOpts ...exitCallbackOpt) {
	l.addCallback(func(ctx context.Context) error {
		return callbackWithContext(ctx, callback)
	}, exitOpts...)
}

// OnExitWithContext registers a callback that will be called when the application is about to exit.
// Has no effect after calling Life.Run().
// The callback will receive a context that will be cancelled after the teardown timeout.
// Use life.Async option to execute the callback in a separate goroutine.
// life.PanicOnError has no effect on this function.
func (l *Life) OnExitWithContext(callback func(context.Context), exitOpts ...exitCallbackOpt) {
	l.addCallback(func(ctx context.Context) error {
		callback(ctx)
		return nil
	}, exitOpts...)
}

// OnExitWithContextError registers a callback that will be called when the application is about to exit.
// Has no effect after calling Life.Run().
// The callback will receive a context that will be cancelled after the teardown timeout.
// The callback can return an error that will be passed to the error handler.
// Use life.Async option to execute the callback in a separate goroutine.
// Use life.PanicOnError to panic with the error returned by the callback.
func (l *Life) OnExitWithContextError(callback func(context.Context) error, exitOpts ...exitCallbackOpt) {
	l.addCallback(callback, exitOpts...)
}

func (l *Life) addCallback(cb func(context.Context) error, exitOpts ...exitCallbackOpt) {
	if l.phase.Load() != int32(phaseStarting) {
		return
	}

	c := &callback{
		fn: cb,
	}

	for _, opt := range exitOpts {
		opt(c)
	}

	l.callbacks = append(l.callbacks, c)
}

// TeardownContext returns a context that will be cancelled after the teardown timeout.
// It can be used to control the shutdown of the application.
// This context is the same as the one passed to the callbacks registered with OnExit* methods.
func (l *Life) TeardownContext() context.Context {
	return l.teardownCtx
}

// Die stops the application.
// It will also block until all the registered callbacks are executed.
// If the teardown timeout is set, it will be used to cancel the context passed to the callbacks.
func (l *Life) Die(reason error) {
	if l.phase.Load() == int32(phaseTeardown) {
		return
	}

	l.phase.Store(int32(phaseTeardown))

	l.runningCancel(reason)

	go func() {
		if l.teardownTimeout > 0 {
			<-time.After(l.teardownTimeout)
			l.teardownCancel()
		}
	}()

	l.exit()
	close(l.die)
}

// Run starts the application. It will block until the application is stopped by calling Die.
func (l *Life) Run() error {
	if !l.phase.CompareAndSwap(int32(phaseStarting), int32(phaseRunning)) {
		panic("Life.Run() called after Life.Die()")
	}

	l.startingCancel()

	<-l.die

	return l.runningCtx.Err()
}

func (l *Life) exit() {
	ctx := l.TeardownContext()

	wg := sync.WaitGroup{}

	for _, cb := range l.callbacks {
		if cb.executeBehaviour != executeAsync {
			continue
		}

		wg.Add(1)
		go func(cb *callback) {
			defer wg.Done()
			if err := cb.fn(ctx); err != nil {
				l.handleExitError(cb.errorBehaviour, err)
			}
		}(cb)
	}

	for _, cb := range l.callbacks {
		if cb.executeBehaviour != executeSync {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := cb.fn(ctx); err != nil {
			l.handleExitError(cb.errorBehaviour, err)
		}
	}

	wg.Wait()
}

func (l *Life) handleExitError(errBehaviour exitBehaviour, err error) {
	if l.errorHandler != nil {
		l.errorHandler(err)
	}

	if errBehaviour == panicOnError {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return // ignore context errors
		}
		if err != nil {
			panic(err)
		}
	}
}

func callbackWithContext(ctx context.Context, callback func() error) error {
	errChan := make(chan error)
	go func() {
		errChan <- callback()
	}()
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
