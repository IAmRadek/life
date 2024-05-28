package life_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/IAmRadek/life"
)

var unexpectedErr = fmt.Errorf("unexpected err")

func TestExitCallbacks(t *testing.T) {
	t.Parallel()

	callsMutex := &sync.Mutex{}
	calls := 0

	l := life.New()

	l.OnExit(func() {
		callsMutex.Lock()
		defer callsMutex.Unlock()
		calls++
	})

	l.OnExitWithError(func() error {
		callsMutex.Lock()
		defer callsMutex.Unlock()
		calls++
		return nil
	})

	l.OnExitWithContext(func(ctx context.Context) {
		callsMutex.Lock()
		defer callsMutex.Unlock()
		calls++
	})

	l.OnExitWithContextError(func(ctx context.Context) error {
		callsMutex.Lock()
		defer callsMutex.Unlock()
		calls++
		return nil
	})

	l.OnExitWithContextError(func(ctx context.Context) error {
		callsMutex.Lock()
		defer callsMutex.Unlock()
		calls++
		return nil
	}, life.Async)

	go func() {
		<-l.StartingContext().Done()
		l.Die(unexpectedErr)
	}()

	if err := l.Run(); !errors.Is(err, unexpectedErr) {
		t.Errorf("expected %q, got: %q", unexpectedErr, err)
	}

	if calls != 5 {
		t.Errorf("expected 4 calls, got %d", calls)
	}
}

func TestPanic(t *testing.T) {
	t.Parallel()

	l := life.New()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	l.OnExitWithContextError(func(ctx context.Context) error {
		return errors.New("test error")
	}, life.PanicOnError)

	l.Die(unexpectedErr)
	if err := l.Run(); !errors.Is(err, unexpectedErr) {
		t.Errorf("expected %q, got: %q", unexpectedErr, err)
	}

	t.Error("The code did not panic")
}

func TestTimeout(t *testing.T) {
	t.Parallel()

	timeout := 10 * time.Millisecond
	timeoutJitter := 5 * time.Millisecond

	l := life.New(life.WithTeardownTimeout(timeout))

	l.OnExit(func() {
		time.Sleep(2 * timeout)
	})

	l.OnExit(func() {
		panic("should not be called")
	})

	go func() {
		<-l.StartingContext().Done()
		l.Die(unexpectedErr)
	}()

	start := time.Now()
	if err := l.Run(); !errors.Is(err, unexpectedErr) {
		t.Errorf("expected %q, got: %q", unexpectedErr, err)
	}
	end := time.Now()

	if end.Sub(start) < timeout-timeoutJitter || end.Sub(start) > timeout+timeoutJitter {
		t.Errorf("expected timeout between %v and %v, got %v", timeout-timeoutJitter, timeout+timeoutJitter, end.Sub(start))
	}
}
