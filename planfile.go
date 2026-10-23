package executor

import (
	"fmt"
	"strings"
)

// ParsePlan reads a text plan. Blank lines and lines starting with # are
// ignored. Every other line has the form:
//
//	<id> <ok|fail|panic> [reason words...] [<- dep,dep]
func ParsePlan(src string) ([]Task, error) {
	var tasks []Task
	for number, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		task, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", number+1, err)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func parseLine(line string) (Task, error) {
	tokens := strings.Fields(line)
	if len(tokens) < 2 {
		return Task{}, fmt.Errorf("expected <id> <ok|fail|panic> [reason] [<- deps]")
	}
	outcome := tokens[1]
	reason, deps, err := splitReasonAndDeps(tokens[2:])
	if err != nil {
		return Task{}, err
	}
	switch outcome {
	case "ok":
		if reason != "" {
			return Task{}, fmt.Errorf("ok does not take a reason")
		}
	case "fail", "panic":
	default:
		return Task{}, fmt.Errorf("unknown outcome %q", outcome)
	}
	return Task{
		ID:           tokens[0],
		Dependencies: deps,
		Action:       scriptAction(outcome, reason),
	}, nil
}

func splitReasonAndDeps(tokens []string) (string, []string, error) {
	arrow := -1
	for i, token := range tokens {
		if token == "<-" {
			if arrow >= 0 {
				return "", nil, fmt.Errorf("more than one <-")
			}
			arrow = i
		}
	}
	reasonTokens := tokens
	var depTokens []string
	if arrow >= 0 {
		reasonTokens = tokens[:arrow]
		depTokens = tokens[arrow+1:]
	}
	var deps []string
	for _, token := range depTokens {
		for _, part := range strings.Split(token, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				deps = append(deps, part)
			}
		}
	}
	return strings.Join(reasonTokens, " "), deps, nil
}

func scriptAction(outcome, reason string) func() error {
	return func() error {
		switch outcome {
		case "ok":
			return nil
		case "fail":
			return fmt.Errorf("%s", reason)
		default:
			if reason == "" {
				panic("action panicked")
			}
			panic(reason)
		}
	}
}

// FormatReport renders a report as plain text.
func FormatReport(report Report) string {
	var b strings.Builder
	b.WriteString("executed:\n")
	if len(report.Executed) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, id := range report.Executed {
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	b.WriteString("tasks:\n")
	if len(report.Tasks) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, task := range report.Tasks {
		fmt.Fprintf(&b, "- id: %s\n", task.ID)
		fmt.Fprintf(&b, "  status: %s\n", task.Status)
		if task.BlockedBy != "" {
			fmt.Fprintf(&b, "  blocked_by: %s\n", task.BlockedBy)
		}
		if task.Reason != "" {
			fmt.Fprintf(&b, "  reason: %s\n", task.Reason)
		}
	}
	return b.String()
}
