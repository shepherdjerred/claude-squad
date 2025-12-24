// Package log provides logging utilities including debug mode with render profiling.
// Enable debug mode by setting CS_DEBUG=1 environment variable.
package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Debug mode configuration
var (
	DebugEnabled bool
	DebugLog     *log.Logger
	debugLogFile *os.File
)

var debugLogFileName = filepath.Join(os.TempDir(), "claudesquad-debug.log")

// InitDebug initializes debug logging if CS_DEBUG=1 is set.
// Call this after Initialize() in main.
func InitDebug() {
	if os.Getenv("CS_DEBUG") != "1" {
		// Initialize DebugLog as a no-op logger to prevent nil pointer panics
		DebugLog = log.New(io.Discard, "", 0)
		return
	}

	DebugEnabled = true

	f, err := os.OpenFile(debugLogFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		if ErrorLog != nil {
			ErrorLog.Printf("could not open debug log file: %s", err)
		}
		// Fall back to no-op logger on error
		DebugLog = log.New(io.Discard, "", 0)
		return
	}

	DebugLog = log.New(f, "DEBUG:", log.Ldate|log.Ltime|log.Lmicroseconds)
	debugLogFile = f

	DebugLog.Println("Debug mode enabled")
	DebugLog.Printf("Debug log: %s", debugLogFileName)
}

// CloseDebug closes the debug log file.
func CloseDebug() {
	if debugLogFile != nil {
		_ = debugLogFile.Close()
		fmt.Println("wrote debug logs to " + debugLogFileName)
	}
}

// Debug logs a debug message if debug mode is enabled.
func Debug(format string, v ...interface{}) {
	if DebugEnabled && DebugLog != nil {
		DebugLog.Printf(format, v...)
	}
}

// RenderProfiler tracks rendering performance metrics.
type RenderProfiler struct {
	mu           sync.RWMutex
	components   map[string]*ComponentMetrics
	frameCount   int64
	totalTime    time.Duration
	lastFrameAt  time.Time
	frameTimings []time.Duration // Rolling window of frame times
}

// ComponentMetrics tracks metrics for a single component.
type ComponentMetrics struct {
	Name         string
	RenderCount  int64
	TotalTime    time.Duration
	MinTime      time.Duration
	MaxTime      time.Duration
	LastRenderAt time.Time
}

// Global profiler instance
var profiler = &RenderProfiler{
	components:   make(map[string]*ComponentMetrics),
	frameTimings: make([]time.Duration, 0, 100),
}

// GetProfiler returns the global render profiler.
func GetProfiler() *RenderProfiler {
	return profiler
}

// StartRender begins timing a component render.
// Returns a function to call when render completes.
func (p *RenderProfiler) StartRender(component string) func() {
	if !DebugEnabled {
		return func() {}
	}

	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		p.recordRender(component, elapsed)
	}
}

// recordRender records a render timing.
func (p *RenderProfiler) recordRender(component string, elapsed time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	metrics, ok := p.components[component]
	if !ok {
		metrics = &ComponentMetrics{
			Name:    component,
			MinTime: elapsed,
			MaxTime: elapsed,
		}
		p.components[component] = metrics
	}

	metrics.RenderCount++
	metrics.TotalTime += elapsed
	metrics.LastRenderAt = time.Now()

	if elapsed < metrics.MinTime {
		metrics.MinTime = elapsed
	}
	if elapsed > metrics.MaxTime {
		metrics.MaxTime = elapsed
	}
}

// RecordFrame records a complete frame render.
func (p *RenderProfiler) RecordFrame(elapsed time.Duration) {
	if !DebugEnabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.frameCount++
	p.totalTime += elapsed
	p.lastFrameAt = time.Now()

	// Keep rolling window of last 100 frame times
	if len(p.frameTimings) >= 100 {
		p.frameTimings = p.frameTimings[1:]
	}
	p.frameTimings = append(p.frameTimings, elapsed)

	// Log slow frames (> 16ms = 60fps threshold)
	if elapsed > 16*time.Millisecond && DebugLog != nil {
		DebugLog.Printf("SLOW FRAME: %v", elapsed)
	}
}

// GetStats returns a summary of render statistics.
func (p *RenderProfiler) GetStats() string {
	if !DebugEnabled {
		return ""
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n=== Render Profile ===\n")
	sb.WriteString(fmt.Sprintf("Total frames: %d\n", p.frameCount))

	if p.frameCount > 0 {
		avgFrame := p.totalTime / time.Duration(p.frameCount)
		sb.WriteString(fmt.Sprintf("Avg frame time: %v\n", avgFrame))
		sb.WriteString(fmt.Sprintf("Theoretical FPS: %.1f\n", 1.0/avgFrame.Seconds()))
	}

	// Recent frame stats
	if len(p.frameTimings) > 0 {
		var sum time.Duration
		min := p.frameTimings[0]
		max := p.frameTimings[0]
		for _, t := range p.frameTimings {
			sum += t
			if t < min {
				min = t
			}
			if t > max {
				max = t
			}
		}
		avg := sum / time.Duration(len(p.frameTimings))
		sb.WriteString(fmt.Sprintf("Recent %d frames: avg=%v min=%v max=%v\n",
			len(p.frameTimings), avg, min, max))
	}

	// Component breakdown
	sb.WriteString("\n--- Components ---\n")

	// Sort by total time descending
	var sorted []*ComponentMetrics
	for _, m := range p.components {
		sorted = append(sorted, m)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TotalTime > sorted[j].TotalTime
	})

	for _, m := range sorted {
		avg := time.Duration(0)
		if m.RenderCount > 0 {
			avg = m.TotalTime / time.Duration(m.RenderCount)
		}
		sb.WriteString(fmt.Sprintf("  %s: count=%d total=%v avg=%v min=%v max=%v\n",
			m.Name, m.RenderCount, m.TotalTime, avg, m.MinTime, m.MaxTime))
	}

	return sb.String()
}

// LogStats logs the current render statistics.
func (p *RenderProfiler) LogStats() {
	if DebugEnabled && DebugLog != nil {
		DebugLog.Print(p.GetStats())
	}
}

// Reset clears all profiling data.
func (p *RenderProfiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.components = make(map[string]*ComponentMetrics)
	p.frameCount = 0
	p.totalTime = 0
	p.frameTimings = make([]time.Duration, 0, 100)
}

// ComponentTrace logs component lifecycle events.
type ComponentTrace struct {
	component string
	startTime time.Time
}

// TraceComponent creates a new component trace.
func TraceComponent(component string) *ComponentTrace {
	if !DebugEnabled {
		return nil
	}
	return &ComponentTrace{
		component: component,
		startTime: time.Now(),
	}
}

// Event logs a component event.
func (t *ComponentTrace) Event(event string, details ...interface{}) {
	if t == nil || !DebugEnabled || DebugLog == nil {
		return
	}

	elapsed := time.Since(t.startTime)
	if len(details) > 0 {
		DebugLog.Printf("[%s] %s (+%v): %v", t.component, event, elapsed, details)
	} else {
		DebugLog.Printf("[%s] %s (+%v)", t.component, event, elapsed)
	}
}

// LayoutTrace logs layout computation events.
func LayoutTrace(format string, v ...interface{}) {
	if DebugEnabled && DebugLog != nil {
		DebugLog.Printf("[LAYOUT] "+format, v...)
	}
}

// RenderTrace logs render events.
func RenderTrace(component, format string, v ...interface{}) {
	if DebugEnabled && DebugLog != nil {
		msg := fmt.Sprintf(format, v...)
		DebugLog.Printf("[RENDER:%s] %s", component, msg)
	}
}

// InputTrace logs input handling events.
func InputTrace(format string, v ...interface{}) {
	if DebugEnabled && DebugLog != nil {
		DebugLog.Printf("[INPUT] "+format, v...)
	}
}

// PerformanceWarning logs performance-related warnings.
func PerformanceWarning(format string, v ...interface{}) {
	if DebugEnabled && DebugLog != nil {
		DebugLog.Printf("[PERF WARNING] "+format, v...)
	}
}

// MemoryStats logs current memory statistics.
func MemoryStats() {
	if !DebugEnabled || DebugLog == nil {
		return
	}

	// Note: For detailed memory stats, could use runtime.ReadMemStats()
	// but that can be slow, so we keep this simple
	DebugLog.Println("[MEMORY] Stats logging requested")
}
