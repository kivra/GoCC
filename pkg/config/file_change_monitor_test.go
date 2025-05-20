package config

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestFileChangeMonitor_Close(t *testing.T) {

	tempDir := os.TempDir() + "/file_change_monitor_test"
	fileName := "test.txt"

	if !DirExists(tempDir) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
	}

	monitor := MonitorJsonFileUpdates(tempDir, fileName, NoTransformer)
	defer monitor.Close()

	// Cant test change count, because on linux, we also get a close event
	// Whiel on macos we only get writes :S
	//changeCount := atomic.Int32{}

	writeToFile := func(content string) {
		filePath := tempDir + "/" + fileName
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write to file: %v", err)
		}
	}

	writeToOtherFile := func(content string) {
		filePath := tempDir + "/" + fileName + "x"
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write to file: %v", err)
		}
	}

	go func() {
		writeToFile("first content")          // not valid json
		time.Sleep(250 * time.Millisecond)    // wait for the file to be processed
		writeToFile(`{"key": "value"}`)       // valid json
		writeToOtherFile(`{"key": "value"}`)  // valid json
		time.Sleep(250 * time.Millisecond)    // wait for the file to be processed
		writeToFile(`{"key": "value2"}`)      // valid json
		writeToOtherFile(`{"key": "value2"}`) // valid json
		time.Sleep(250 * time.Millisecond)    // wait for the file to be processed
		writeToFile("{ ")                     // not valid json
		writeToOtherFile("{ ")                // not valid json
		time.Sleep(250 * time.Millisecond)    // wait for the file to be processed
		writeToOtherFile(`{"key": "value4"}`) // valid json
		time.Sleep(250 * time.Millisecond)    // wait for the file to be processed
		writeToFile(`{"key": "value3"}`)      // valid json
	}()

	lastReceived := ""
	t0 := time.Now()
	for time.Since(t0) < 10*time.Second && lastReceived == "" {
		select {
		case change := <-monitor.Changes():
			//changeCount.Add(1)
			slog.Info("File content changed to: " + change)
			if change == `{"key": "value3"}` {
				slog.Info("Received all updates")
				lastReceived = change
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Timed out waiting for file changes")
		}
	}

	if lastReceived != `{"key": "value3"}` {
		t.Fatalf("Did not receive the correct last change")
	}

	// Cant test change count, because on linux, we also get a close event
	// Whiel on macos we only get writes :S
	//if changeCount.Load() > 3 {
	//	t.Fatalf("Received more changes than expected")
	//}
}
