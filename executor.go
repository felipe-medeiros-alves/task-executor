// Package executor runs a task plan one action at a time, in ascending
// identifier order, after the whole plan has been validated.
//
// Actions are ordinary functions. The package does not start processes.
package executor

import (
	"fmt"
	"sort"
)

// Status is the outcome of one task after a plan runs.
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusBlocked Status = "blocked"
)

// Task is one unit of work. Dependencies are identifiers that must succeed
// before Action runs.
type Task struct {
	ID           string
	Dependencies []string
	Action       func() error
}

// TaskState is the final state of one task. BlockedBy is set only when
// Status is blocked, and it is the smallest direct dependency that failed
// or was itself blocked.
type TaskState struct {
	ID        string
	Status    Status
	Reason    string
	BlockedBy string
}

// Report is the result of one execution. Executed lists only actions that
// ran, in that order. Tasks lists every task, sorted by identifier.
type Report struct {
	Executed []string
	Tasks    []TaskState
}

// Execute validates tasks and, only if the plan is valid, runs it.
// A rejected plan runs no actions. An empty plan is valid and runs nothing.
// Each call starts from scratch.
func Execute(tasks []Task) (Report, error) {
	if err := validate(tasks); err != nil {
		return Report{}, err
	}

	byID := make(map[string]Task, len(tasks))
	pending := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
		pending[task.ID] = struct{}{}
	}

	status := make(map[string]Status, len(tasks))
	reason := make(map[string]string, len(tasks))
	blockedBy := make(map[string]string, len(tasks))
	executed := make([]string, 0, len(tasks))

	for len(pending) > 0 {
		closeBlocked(pending, byID, status, reason, blockedBy)
		if len(pending) == 0 {
			break
		}
		ready := readyIDs(pending, byID, status)
		if len(ready) == 0 {
			return Report{}, fmt.Errorf("executor stuck with pending tasks %v", sortedIDs(pending))
		}
		id := ready[0]
		state, why := runAction(byID[id].Action)
		status[id] = state
		reason[id] = why
		executed = append(executed, id)
		delete(pending, id)
	}

	return Report{Executed: executed, Tasks: states(byID, status, reason, blockedBy)}, nil
}

// closeBlocked marks every task that can no longer succeed, including tasks
// that only become blocked after another task in this wave is marked.
func closeBlocked(pending map[string]struct{}, byID map[string]Task, status map[string]Status, reason, blockedBy map[string]string) {
	for {
		marked := false
		for _, id := range sortedIDs(pending) {
			blocker, ok := smallestBlockingDependency(byID[id].Dependencies, status)
			if !ok {
				continue
			}
			status[id] = StatusBlocked
			blockedBy[id] = blocker
			reason[id] = fmt.Sprintf("blocked by %s (%s)", blocker, status[blocker])
			delete(pending, id)
			marked = true
		}
		if !marked {
			return
		}
	}
}

func readyIDs(pending map[string]struct{}, byID map[string]Task, status map[string]Status) []string {
	ready := make([]string, 0, len(pending))
	for id := range pending {
		if allSucceeded(byID[id].Dependencies, status) {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	return ready
}

func allSucceeded(deps []string, status map[string]Status) bool {
	for _, dep := range deps {
		if status[dep] != StatusSuccess {
			return false
		}
	}
	return true
}

func smallestBlockingDependency(deps []string, status map[string]Status) (string, bool) {
	blocker := ""
	for _, dep := range deps {
		if status[dep] != StatusFailed && status[dep] != StatusBlocked {
			continue
		}
		if blocker == "" || dep < blocker {
			blocker = dep
		}
	}
	return blocker, blocker != ""
}

func states(byID map[string]Task, status map[string]Status, reason, blockedBy map[string]string) []TaskState {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]TaskState, 0, len(ids))
	for _, id := range ids {
		out = append(out, TaskState{
			ID:        id,
			Status:    status[id],
			Reason:    reason[id],
			BlockedBy: blockedBy[id],
		})
	}
	return out
}

func sortedIDs(ids map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// runAction treats a panic as a failed task. The case says a thrown
// exception fails that task and must not abort the rest of the plan.
func runAction(action func() error) (Status, string) {
	var err error
	var panicValue any
	panicked := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				panicked = true
				panicValue = recovered
			}
		}()
		err = action()
	}()
	if panicked {
		return StatusFailed, fmt.Sprintf("panic: %v", panicValue)
	}
	if err != nil {
		message := err.Error()
		if message == "" {
			message = "action failed"
		}
		return StatusFailed, message
	}
	return StatusSuccess, ""
}
