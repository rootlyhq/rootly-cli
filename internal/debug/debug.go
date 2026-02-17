package debug

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/log"
)

// ringBuffer is a thread-safe circular buffer for log entries
type ringBuffer struct {
	mu      sync.RWMutex
	entries []string
	maxSize int
	pos     int
	full    bool
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		entries: make([]string, size),
		maxSize: size,
	}
}

func (r *ringBuffer) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := string(p)
	r.entries[r.pos] = entry
	r.pos = (r.pos + 1) % r.maxSize
	if r.pos == 0 {
		r.full = true
	}
	return len(p), nil
}

func (r *ringBuffer) GetEntries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.full {
		return r.entries[:r.pos]
	}

	// Return entries in order (oldest first)
	result := make([]string, r.maxSize)
	copy(result, r.entries[r.pos:])
	copy(result[r.maxSize-r.pos:], r.entries[:r.pos])
	return result
}

func (r *ringBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make([]string, r.maxSize)
	r.pos = 0
	r.full = false
}

var (
	// LogBuffer stores recent log entries in memory
	LogBuffer = newRingBuffer(1000)

	// Logger is the global debug logger
	Logger *log.Logger

	// Enabled indicates if debug mode is active
	Enabled bool

	// fileOutput holds file writer if logging to file
	fileOutput io.Writer

	// LogFilePath is the path to the log file (if set via --log)
	LogFilePath string
)

func init() {
	// Create logger that writes to buffer (always) and optionally to stderr/file
	Logger = log.NewWithOptions(LogBuffer, log.Options{
		ReportTimestamp: true,
		Prefix:          "rootly-tui",
		Level:           log.DebugLevel, // Always capture to buffer
	})
}

// PrettyJSON formats JSON bytes for readable logging
func PrettyJSON(data []byte) string {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "  "); err != nil {
		return string(data) // Return raw if indent fails
	}
	return prettyJSON.String()
}

// Enable turns on debug logging to stderr
func Enable() {
	Enabled = true
	if fileOutput != nil {
		Logger.SetOutput(io.MultiWriter(LogBuffer, fileOutput))
	} else {
		Logger.SetOutput(io.MultiWriter(LogBuffer, os.Stderr))
	}
	Logger.Debug("Debug mode enabled")
}

// Disable turns off debug logging to stderr (still logs to buffer)
func Disable() {
	Enabled = false
	Logger.SetOutput(LogBuffer)
}

// SetLogFile writes logs to a file in addition to buffer
func SetLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	fileOutput = f
	LogFilePath = path
	Logger.SetOutput(io.MultiWriter(LogBuffer, f))
	return nil
}

// MaxLogLines is the maximum number of lines to read from log file
const MaxLogLines = 1000

// ReadLogFile reads the last MaxLogLines lines from the log file
func ReadLogFile() (string, error) {
	if LogFilePath == "" {
		return "", nil
	}

	f, err := os.Open(LogFilePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	// Get file size
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	// For small files, read the whole thing
	fileSize := stat.Size()
	if fileSize < 100*1024 { // Less than 100KB
		data, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// For larger files, read from the end (tail)
	// Start with last 100KB and increase if needed
	bufSize := int64(100 * 1024)
	if bufSize > fileSize {
		bufSize = fileSize
	}

	// Seek to near end
	_, err = f.Seek(-bufSize, io.SeekEnd)
	if err != nil {
		// If seek fails, read from start
		_, _ = f.Seek(0, io.SeekStart)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	content := string(data)

	// Split into lines and keep only the last MaxLogLines
	lines := splitLines(content)
	if len(lines) > MaxLogLines {
		lines = lines[len(lines)-MaxLogLines:]
	}

	return joinLines(lines), nil
}

// splitLines splits content into lines, handling both \n and \r\n
func splitLines(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			line := content[start:i]
			if line != "" && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	// Handle last line without newline
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// joinLines joins lines back with newlines
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b bytes.Buffer
	for i, line := range lines {
		b.WriteString(line)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// HasLogFile returns true if logging to a file
func HasLogFile() bool {
	return LogFilePath != ""
}

// GetLogs returns all log entries from the buffer
func GetLogs() []string {
	return LogBuffer.GetEntries()
}

// ClearLogs clears the log buffer
func ClearLogs() {
	LogBuffer.Clear()
}
