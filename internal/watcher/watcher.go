package watcher

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event represents a file system event.
type Event struct {
	Name string
	Path string
	At   time.Time
}

// Watcher monitors a file or directory for modification and creation events.
type Watcher struct {
	fs     *fsnotify.Watcher
	target string
	Events chan Event
	done   chan struct{}
}

// New creates a new Watcher for target path (directory or file).
func New(target string) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed creating fsnotify watcher: %w", err)
	}

	w := &Watcher{
		fs:     fs,
		target: target,
		Events: make(chan Event, 10),
		done:   make(chan struct{}),
	}

	// If target is a file that might not exist yet, watch its parent directory
	watchPath := target
	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			watchPath = filepath.Dir(target)
		}
	} else {
		// Target does not exist yet, watch parent directory if it exists
		parent := filepath.Dir(target)
		if _, err := os.Stat(parent); err == nil {
			watchPath = parent
		}
	}

	if err := w.fs.Add(watchPath); err != nil {
		w.fs.Close()
		return nil, fmt.Errorf("failed adding watch path %s: %w", watchPath, err)
	}

	slog.Debug("watcher started", "target", target, "watching", watchPath)
	return w, nil
}

// Watch starts listening for file system events in a background goroutine.
func (w *Watcher) Watch() {
	go func() {
		defer close(w.Events)
		for {
			select {
			case <-w.done:
				return
			case event, ok := <-w.fs.Events:
				if !ok {
					return
				}
				slog.Debug("fsnotify event", "op", event.Op, "name", event.Name)
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					baseName := filepath.Base(event.Name)
					targetBase := filepath.Base(w.target)

					// On Windows, file systems are case-insensitive
					if targetBase != "" && targetBase != "." && !strings.EqualFold(targetBase, baseName) {
						continue
					}

					select {
					case w.Events <- Event{Name: baseName, Path: event.Name, At: time.Now()}:
					case <-w.done:
						return
					}
				}
			case err, ok := <-w.fs.Errors:
				if !ok {
					return
				}
				slog.Error("fsnotify error", "error", err)
			}
		}
	}()
}

// Close stops the watcher and frees resources.
func (w *Watcher) Close() error {
	select {
	case <-w.done:
		// already closed
		return nil
	default:
		close(w.done)
	}
	return w.fs.Close()
}
