package reader

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"ed-assist/internal/parser"
	"ed-assist/internal/watcher"
)

// Mode defines the update detection mechanism.
type Mode string

const (
	// ModeWatch uses OS file system notifications (fsnotify) - 0% CPU when idle.
	ModeWatch Mode = "watch"
	// ModePoll polls the file at a fixed interval (e.g. every 250ms).
	ModePoll Mode = "poll"
)

// Reader reads Status.json either on file-system events (watch mode) or periodically (poll mode),
// with quick retry logic to handle file locking and mid-write states.
type Reader struct {
	filePath     string
	mode         Mode
	pollInterval time.Duration
	maxRetries   int
	retryDelay   time.Duration

	mu           sync.RWMutex
	lastStatus   *parser.Status
	lastHash     [32]byte
	fileNotFound bool

	statusChan chan *parser.Status
	stopChan   chan struct{}
	doneChan   chan struct{}
}

// Option configures the Reader.
type Option func(*Reader)

// WithMode sets the reader operation mode (ModeWatch or ModePoll).
func WithMode(m Mode) Option {
	return func(r *Reader) {
		if m == ModeWatch || m == ModePoll {
			r.mode = m
		}
	}
}

// WithPollInterval sets the polling interval.
func WithPollInterval(d time.Duration) Option {
	return func(r *Reader) {
		if d > 0 {
			r.pollInterval = d
		}
	}
}

// WithRetries sets the maximum number of quick retries and delay between them.
func WithRetries(maxRetries int, retryDelay time.Duration) Option {
	return func(r *Reader) {
		if maxRetries >= 0 {
			r.maxRetries = maxRetries
		}
		if retryDelay > 0 {
			r.retryDelay = retryDelay
		}
	}
}

// New creates a new status Reader.
func New(filePath string, opts ...Option) *Reader {
	r := &Reader{
		filePath:     filePath,
		mode:         ModeWatch,               // Event-driven by default (0% CPU when idle)
		pollInterval: 250 * time.Millisecond, // 1/4 second default when polling
		maxRetries:   3,
		retryDelay:   25 * time.Millisecond,
		statusChan:   make(chan *parser.Status, 10),
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// StatusChan returns a receive-only channel that emits updated Status records.
func (r *Reader) StatusChan() <-chan *parser.Status {
	return r.statusChan
}

// LastStatus returns the most recently parsed Status, or nil if none.
func (r *Reader) LastStatus() *parser.Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastStatus
}

// ReadOnce attempts to read and parse the status file once, performing quick retries on failure.
func (r *Reader) ReadOnce() (*parser.Status, error) {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(r.retryDelay)
		}

		data, err := os.ReadFile(r.filePath)
		if err != nil {
			lastErr = err
			// If file does not exist, quick retries within milliseconds won't help.
			if os.IsNotExist(err) {
				return nil, err
			}
			continue
		}

		if len(data) == 0 {
			lastErr = errors.New("file is empty (possibly being written)")
			continue
		}

		status, err := parser.Parse(data)
		if err != nil {
			lastErr = err
			continue
		}

		// Success!
		if attempt > 0 {
			slog.Debug("read Status.json succeeded after retry", "attempt", attempt, "file", r.filePath)
		}
		return status, nil
	}

	return nil, fmt.Errorf("failed to read Status.json after %d retries: %w", r.maxRetries, lastErr)
}

// Start runs the reader loop (watching or polling) until ctx is canceled or Stop() is called.
func (r *Reader) Start(ctx context.Context) {
	defer close(r.doneChan)
	defer close(r.statusChan)

	if r.mode == ModePoll {
		r.runPollLoop(ctx)
		return
	}

	w, err := watcher.New(r.filePath)
	if err != nil {
		slog.Warn("failed to initialize file watcher, falling back to polling", "error", err)
		r.runPollLoop(ctx)
		return
	}
	defer w.Close()
	w.Watch()

	slog.Info("status reader started in watch mode (notification-driven)", "file", r.filePath)

	// Perform initial read immediately so current state is displayed on startup
	r.pollTick()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("status reader stopping due to context cancel")
			return
		case <-r.stopChan:
			slog.Debug("status reader stopping due to Stop() call")
			return
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			r.pollTick()
		}
	}
}

func (r *Reader) runPollLoop(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	slog.Info("status reader started in polling mode", "poll_interval", r.pollInterval, "file", r.filePath)

	// Perform initial read immediately
	r.pollTick()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("status reader stopping due to context cancel")
			return
		case <-r.stopChan:
			slog.Debug("status reader stopping due to Stop() call")
			return
		case <-ticker.C:
			r.pollTick()
		}
	}
}

func (r *Reader) pollTick() {
	status, err := r.ReadOnce()
	if err != nil {
		if os.IsNotExist(err) {
			r.mu.Lock()
			if !r.fileNotFound {
				slog.Info("waiting for Status.json to be created by Elite Dangerous...", "path", r.filePath)
				r.fileNotFound = true
			}
			r.mu.Unlock()
			return
		}
		slog.Warn("error reading status file", "error", err, "path", r.filePath)
		return
	}

	r.mu.Lock()
	if r.fileNotFound {
		slog.Info("Status.json found and accessible!", "path", r.filePath)
		r.fileNotFound = false
	}

	// Compute hash of timestamp + raw flags + pips to detect updates
	hashKey := fmt.Sprintf("%s|%d|%d|%v|%v|%v|%v",
		status.Timestamp,
		status.RawFlags,
		status.RawFlags2,
		status.GuiFocus,
		status.Pips,
		status.Fuel,
		status.Altitude,
	)
	currentHash := sha256.Sum256([]byte(hashKey))

	if currentHash == r.lastHash {
		r.mu.Unlock()
		return
	}

	r.lastHash = currentHash
	r.lastStatus = status
	r.mu.Unlock()

	select {
	case r.statusChan <- status:
	default:
		// Drop older unconsumed status if channel buffer full
		select {
		case <-r.statusChan:
		default:
		}
		r.statusChan <- status
	}
}

// Stop signals the reader to stop.
func (r *Reader) Stop() {
	select {
	case <-r.stopChan:
		// already stopping
	default:
		close(r.stopChan)
	}
}

// Wait blocks until the reader has completely shut down.
func (r *Reader) Wait() {
	<-r.doneChan
}
