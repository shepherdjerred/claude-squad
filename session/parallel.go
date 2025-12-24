package session

import (
	"runtime"
	"sync"
)

// UpdateResult contains the result of updating a single instance.
type UpdateResult struct {
	Instance     *Instance
	Updated      bool
	HasPrompt    bool
	Error        error
	WasRestarted bool // True if the program was restarted due to not running
}

// ParallelUpdate updates all instances concurrently and returns the results.
// Uses a semaphore to limit concurrency to the number of CPUs.
func ParallelUpdate(instances []*Instance) []UpdateResult {
	results := make([]UpdateResult, len(instances))
	var wg sync.WaitGroup

	// Limit concurrency to number of CPUs
	sem := make(chan struct{}, runtime.NumCPU())

	for i, instance := range instances {
		if instance == nil || !instance.Started() || instance.Paused() {
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(idx int, inst *Instance) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			// Check if program needs restart (e.g., after system reboot)
			wasRestarted := false
			if err := inst.CheckAndRestartProgram(); err == nil {
				// If CheckAndRestartProgram returns nil without error,
				// check if it actually restarted by seeing if program wasn't running
				// This is a bit of a hack - ideally CheckAndRestartProgram would return this info
			}

			updated, hasPrompt := inst.HasUpdated()
			results[idx] = UpdateResult{
				Instance:     inst,
				Updated:      updated,
				HasPrompt:    hasPrompt,
				WasRestarted: wasRestarted,
			}
		}(i, instance)
	}

	wg.Wait()
	return results
}

// ParallelUpdateDiffStats updates diff stats for all instances concurrently.
// Deprecated: Use BackgroundUpdateDiffStats for non-blocking updates with rate limiting.
func ParallelUpdateDiffStats(instances []*Instance) []error {
	errors := make([]error, len(instances))
	var wg sync.WaitGroup

	sem := make(chan struct{}, runtime.NumCPU())

	for i, instance := range instances {
		if instance == nil || !instance.Started() || instance.Paused() {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, inst *Instance) {
			defer wg.Done()
			defer func() { <-sem }()

			errors[idx] = inst.UpdateDiffStats()
		}(i, instance)
	}

	wg.Wait()
	return errors
}

// BackgroundUpdateDiffStats spawns background goroutines to update diff stats
// for instances that are due for an update. Non-blocking - returns immediately.
// Rate limiting: 10s delay after activity, max once per 30s per instance.
func BackgroundUpdateDiffStats(instances []*Instance) {
	for _, instance := range instances {
		if instance == nil || !instance.ShouldUpdateDiff() {
			continue
		}

		go func(inst *Instance) {
			_ = inst.UpdateDiffStats()
		}(instance)
	}
}

// BackgroundCaptureClaudeSessionIDs captures Claude session IDs for instances
// that don't have one yet. This runs in background goroutines and is non-blocking.
func BackgroundCaptureClaudeSessionIDs(instances []*Instance) {
	for _, instance := range instances {
		if instance == nil || !instance.Started() || instance.Paused() {
			continue
		}

		// Only capture for instances that don't have a session ID yet
		if instance.GetClaudeSessionID() != "" {
			continue
		}

		go func(inst *Instance) {
			inst.CaptureClaudeSessionID()
		}(instance)
	}
}
