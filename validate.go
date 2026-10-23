package executor

import (
	"fmt"
	"sort"
	"strings"
)

// IssueKind classifies a problem that rejects a plan before any action runs.
type IssueKind string

const (
	IssueInvalidTask       IssueKind = "invalid_task"
	IssueMissingDependency IssueKind = "missing_dependency"
	IssueSelfDependency    IssueKind = "self_dependency"
	IssueCycle             IssueKind = "cycle"
)

// Issue is one reason a plan was rejected.
type Issue struct {
	Kind    IssueKind
	TaskID  string
	Message string
}

// PlanError rejects an entire plan. No action from that plan has run.
type PlanError struct {
	Issues []Issue
}

func (e *PlanError) Error() string {
	var b strings.Builder
	b.WriteString("invalid plan:")
	for _, issue := range e.Issues {
		b.WriteString("\n- ")
		b.WriteString(string(issue.Kind))
		if issue.TaskID != "" {
			fmt.Fprintf(&b, " [%s]", issue.TaskID)
		}
		fmt.Fprintf(&b, ": %s", issue.Message)
	}
	return b.String()
}

func validate(tasks []Task) error {
	var issues []Issue
	counts := map[string]int{}
	wellFormed := true

	for _, task := range tasks {
		if task.ID == "" {
			wellFormed = false
			issues = append(issues, Issue{Kind: IssueInvalidTask, Message: "empty task id"})
			continue
		}
		counts[task.ID]++
		if task.Action == nil {
			issues = append(issues, Issue{Kind: IssueInvalidTask, TaskID: task.ID, Message: "nil action"})
		}
		seenDep := map[string]struct{}{}
		for _, dep := range task.Dependencies {
			if dep == "" {
				issues = append(issues, Issue{Kind: IssueInvalidTask, TaskID: task.ID, Message: "empty dependency id"})
				continue
			}
			if _, ok := seenDep[dep]; ok {
				issues = append(issues, Issue{Kind: IssueInvalidTask, TaskID: task.ID, Message: "duplicate dependency " + dep})
			}
			seenDep[dep] = struct{}{}
			if dep == task.ID {
				issues = append(issues, Issue{Kind: IssueSelfDependency, TaskID: task.ID, Message: "task depends on itself"})
			}
		}
	}

	for id, count := range counts {
		if count > 1 {
			wellFormed = false
			issues = append(issues, Issue{Kind: IssueInvalidTask, TaskID: id, Message: "duplicate task id"})
		}
	}

	for _, task := range tasks {
		if task.ID == "" || counts[task.ID] != 1 {
			continue
		}
		seenDep := map[string]struct{}{}
		for _, dep := range task.Dependencies {
			if dep == "" || dep == task.ID {
				continue
			}
			if _, ok := seenDep[dep]; ok {
				continue
			}
			seenDep[dep] = struct{}{}
			if _, ok := counts[dep]; !ok {
				issues = append(issues, Issue{
					Kind:    IssueMissingDependency,
					TaskID:  task.ID,
					Message: "missing dependency " + dep,
				})
			}
		}
	}

	if wellFormed {
		issues = append(issues, findCycles(tasks)...)
	}
	if len(issues) == 0 {
		return nil
	}
	sort.Slice(issues, func(i, j int) bool {
		if rank(issues[i].Kind) != rank(issues[j].Kind) {
			return rank(issues[i].Kind) < rank(issues[j].Kind)
		}
		if issues[i].TaskID != issues[j].TaskID {
			return issues[i].TaskID < issues[j].TaskID
		}
		return issues[i].Message < issues[j].Message
	})
	return &PlanError{Issues: issues}
}

func rank(kind IssueKind) int {
	switch kind {
	case IssueInvalidTask:
		return 0
	case IssueMissingDependency:
		return 1
	case IssueSelfDependency:
		return 2
	case IssueCycle:
		return 3
	default:
		return 4
	}
}

func findCycles(tasks []Task) []Issue {
	nodes := map[string]struct{}{}
	adj := map[string][]string{}
	for _, task := range tasks {
		if task.ID == "" {
			continue
		}
		nodes[task.ID] = struct{}{}
	}
	for id := range nodes {
		adj[id] = nil
	}
	for _, task := range tasks {
		if _, ok := nodes[task.ID]; !ok {
			continue
		}
		seen := map[string]struct{}{}
		for _, dep := range task.Dependencies {
			if dep == task.ID {
				continue
			}
			if _, ok := nodes[dep]; !ok {
				continue
			}
			if _, ok := seen[dep]; ok {
				continue
			}
			seen[dep] = struct{}{}
			adj[dep] = append(adj[dep], task.ID)
		}
	}

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
		sort.Strings(adj[id])
	}
	sort.Strings(ids)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var issues []Issue
	seenCycle := map[string]struct{}{}

	var visit func(string)
	visit = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		for _, next := range adj[id] {
			switch color[next] {
			case white:
				visit(next)
			case gray:
				start := 0
				for i := range stack {
					if stack[i] == next {
						start = i
						break
					}
				}
				message := canonicalCycle(append(append([]string{}, stack[start:]...), next))
				if _, ok := seenCycle[message]; ok {
					break
				}
				seenCycle[message] = struct{}{}
				issues = append(issues, Issue{
					Kind:    IssueCycle,
					TaskID:  strings.Split(message, " -> ")[0],
					Message: message,
				})
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}

	for _, id := range ids {
		if color[id] == white {
			visit(id)
		}
	}
	return issues
}

func canonicalCycle(path []string) string {
	nodes := path[:len(path)-1]
	minAt := 0
	for i, node := range nodes {
		if node < nodes[minAt] {
			minAt = i
		}
	}
	rotated := append(append([]string{}, nodes[minAt:]...), nodes[:minAt]...)
	rotated = append(rotated, rotated[0])
	return strings.Join(rotated, " -> ")
}
