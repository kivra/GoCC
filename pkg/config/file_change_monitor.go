package config

import (
	"encoding/json"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"log/slog"
	"os"
	"path/filepath"
)

type JsonFileChangeMonitor[T any] struct {
	changeCh <-chan T
	closer   chan any
}

func (f *JsonFileChangeMonitor[T]) Close() {
	if f == nil {
		return
	}
	close(f.closer)
}

func (f *JsonFileChangeMonitor[T]) Changes() <-chan T {
	if f == nil {
		return nil
	}
	return f.changeCh
}

func NoTransformer(in string) (string, bool) {
	return in, true
}

// MonitorJsonFileUpdates monitors a config file dir for changes and reloads the configuration
func MonitorJsonFileUpdates[T any](
	configFileDir string,
	fileName string,
	transformer func(string) (T, bool),
) *JsonFileChangeMonitor[T] {

	// check if the path is a directory
	if !DirExists(configFileDir) {
		panic(fmt.Sprintf("Path does not exist or is not an accessible directory: %v", configFileDir))
	}

	// set up file system watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(fmt.Sprintf("Failed to create watcher: %v", err))
	}

	err = watcher.Add(configFileDir)
	if err != nil {
		panic(fmt.Sprintf("Failed to add path %v to watcher: %v", configFileDir, err))
	}

	changeCh := make(chan string, 10)
	closer := make(chan any)

	// start watching for changes
	go func() {
		defer close(changeCh)
		for {
			select {
			case _, ok := <-closer:
				if !ok {
					slog.Warn(fmt.Sprintf("File change monitor closed for %v", configFileDir))
					return
				}
			case event, ok := <-watcher.Events:
				if !ok {
					panic("BUG: Watcher events channel closed. This should not happen.")
				}

				if event.Op&fsnotify.Write == fsnotify.Write {

					_, fileNameInEvent := filepath.Split(event.Name)
					if fileName != fileNameInEvent {
						slog.Warn(fmt.Sprintf("File %v is not the config file we are looking for, skipping", event.Name))
						continue
					}

					// read the entire file, if it is more than 10 bytes and valid json, emit a change
					bytes, err := os.ReadFile(event.Name)
					if err != nil {
						slog.Warn(fmt.Sprintf("Failed to read file %v: %v", event.Name, err))
						continue
					}

					if len(bytes) < 10 {
						slog.Warn(fmt.Sprintf("File %v is too small to be a valid config file", event.Name))
						continue
					}

					// check if the file is a json file
					testObj := make(map[string]any)
					err = json.Unmarshal(bytes, &testObj)
					if err != nil {
						slog.Warn(fmt.Sprintf("File %v is not a valid json file, skipping", event.Name))
						continue
					}

					slog.Info(fmt.Sprintf("Detected change in file %v, emitting change event", event.Name))
					changeCh <- string(bytes)
				}
			}
		}
	}()

	changeChT := transformChan(changeCh, transformer)

	return &JsonFileChangeMonitor[T]{
		changeCh: changeChT,
		closer:   closer,
	}
}

func DirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fi.IsDir()
}

func transformChan[From any, To any](
	input <-chan From,
	transformer func(From) (to To, keep bool),
) <-chan To {
	output := make(chan To, cap(input))
	go func() {
		defer close(output)
		for in := range input {
			out, keep := transformer(in)
			if keep {
				output <- out
			}
		}
	}()
	return output
}
