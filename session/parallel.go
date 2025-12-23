package session

import (
	"runtime"
	"sync"
)

// UpdateResult contains the result of updating a single instance.
type UpdateResult struct {
	Instance  *Instance
	Updated   bool
	HasPrompt bool
	Error     error
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

			updated, hasPrompt := inst.HasUpdated()
			results[idx] = UpdateResult{
				Instance:  inst,
				Updated:   updated,
				HasPrompt: hasPrompt,
			}
		}(i, instance)
	}

	wg.Wait()
	return results
}

// ParallelUpdateDiffStats updates diff stats for all instances concurrently.
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
