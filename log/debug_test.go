package log

import (
	"os"
	"testing"
	"time"
)

func TestDebugDisabledByDefault(t *testing.T) {
	// Clean up any previous state
	DebugEnabled = false
	DebugLog = nil

	// Without CS_DEBUG=1, debug should be disabled
	os.Unsetenv("CS_DEBUG")
	InitDebug()

	if DebugEnabled {
		t.Error("Debug should be disabled by default")
	}
}

func TestDebugEnabledWithEnvVar(t *testing.T) {
	// Clean up any previous state
	DebugEnabled = false
	DebugLog = nil

	// Set the environment variable
	os.Setenv("CS_DEBUG", "1")
	defer os.Unsetenv("CS_DEBUG")

	InitDebug()
	defer CloseDebug()

	if !DebugEnabled {
		t.Error("Debug should be enabled with CS_DEBUG=1")
	}
	if DebugLog == nil {
		t.Error("DebugLog should be initialized")
	}
}

func TestDebugFunction(t *testing.T) {
	// When disabled, should not panic
	DebugEnabled = false
	DebugLog = nil
	Debug("test message %s", "arg") // Should not panic

	// When enabled but log is nil, should not panic
	DebugEnabled = true
	DebugLog = nil
	Debug("test message %s", "arg") // Should not panic
}

func TestRenderProfiler(t *testing.T) {
	// Reset profiler
	profiler.Reset()

	t.Run("StartRender returns noop when disabled", func(t *testing.T) {
		DebugEnabled = false
		done := profiler.StartRender("test")
		done() // Should not panic or record anything

		if len(profiler.components) != 0 {
			t.Error("Should not record when disabled")
		}
	})

	t.Run("StartRender records when enabled", func(t *testing.T) {
		DebugEnabled = true
		profiler.Reset()

		done := profiler.StartRender("testComponent")
		time.Sleep(1 * time.Millisecond) // Small delay to ensure measurable time
		done()

		if len(profiler.components) != 1 {
			t.Errorf("Expected 1 component, got %d", len(profiler.components))
		}

		metrics := profiler.components["testComponent"]
		if metrics == nil {
			t.Fatal("Expected metrics for testComponent")
		}
		if metrics.RenderCount != 1 {
			t.Errorf("Expected render count 1, got %d", metrics.RenderCount)
		}
		if metrics.TotalTime < time.Millisecond {
			t.Errorf("Expected total time >= 1ms, got %v", metrics.TotalTime)
		}
	})

	t.Run("multiple renders accumulate", func(t *testing.T) {
		DebugEnabled = true
		profiler.Reset()

		for i := 0; i < 5; i++ {
			done := profiler.StartRender("multiComponent")
			done()
		}

		metrics := profiler.components["multiComponent"]
		if metrics == nil {
			t.Fatal("Expected metrics for multiComponent")
		}
		if metrics.RenderCount != 5 {
			t.Errorf("Expected render count 5, got %d", metrics.RenderCount)
		}
	})
}

func TestRecordFrame(t *testing.T) {
	profiler.Reset()
	DebugEnabled = true

	profiler.RecordFrame(10 * time.Millisecond)
	profiler.RecordFrame(20 * time.Millisecond)

	if profiler.frameCount != 2 {
		t.Errorf("Expected frame count 2, got %d", profiler.frameCount)
	}
	if profiler.totalTime != 30*time.Millisecond {
		t.Errorf("Expected total time 30ms, got %v", profiler.totalTime)
	}
}

func TestGetStats(t *testing.T) {
	profiler.Reset()
	DebugEnabled = true

	// Record some data
	profiler.RecordFrame(10 * time.Millisecond)
	done := profiler.StartRender("testComponent")
	done()

	stats := profiler.GetStats()
	if stats == "" {
		t.Error("Expected non-empty stats")
	}

	// Check for expected content
	if !contains(stats, "Render Profile") {
		t.Error("Expected 'Render Profile' in stats")
	}
	if !contains(stats, "testComponent") {
		t.Error("Expected 'testComponent' in stats")
	}
}

func TestComponentTrace(t *testing.T) {
	DebugEnabled = false
	trace := TraceComponent("test")
	if trace != nil {
		t.Error("Expected nil trace when disabled")
	}

	DebugEnabled = true
	trace = TraceComponent("test")
	if trace == nil {
		t.Error("Expected non-nil trace when enabled")
	}

	// Event should not panic even without log
	DebugLog = nil
	trace.Event("test event")
	trace.Event("test event with details", "detail1", "detail2")
}

func TestTraceHelpers(t *testing.T) {
	// All trace helpers should not panic when disabled
	DebugEnabled = false
	DebugLog = nil

	LayoutTrace("test %s", "arg")
	RenderTrace("component", "test %s", "arg")
	InputTrace("test %s", "arg")
	PerformanceWarning("test %s", "arg")
	MemoryStats()

	// Should not panic when enabled but log is nil
	DebugEnabled = true
	DebugLog = nil

	LayoutTrace("test %s", "arg")
	RenderTrace("component", "test %s", "arg")
	InputTrace("test %s", "arg")
	PerformanceWarning("test %s", "arg")
	MemoryStats()
}

func TestRollingWindow(t *testing.T) {
	profiler.Reset()
	DebugEnabled = true

	// Record more than 100 frames
	for i := 0; i < 150; i++ {
		profiler.RecordFrame(time.Millisecond)
	}

	if len(profiler.frameTimings) != 100 {
		t.Errorf("Expected 100 frame timings (rolling window), got %d", len(profiler.frameTimings))
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
