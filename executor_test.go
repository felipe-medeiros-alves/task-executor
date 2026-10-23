package executor_test

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/felipe-medeiros-alves/task-executor"
)

func TestEmptyPlanRunsNothing(t *testing.T) {
	for _, tasks := range [][]executor.Task{nil, {}} {
		report, err := executor.Execute(tasks)
		if err != nil {
			t.Fatalf("empty plan: %v", err)
		}
		if len(report.Executed) != 0 || len(report.Tasks) != 0 {
			t.Fatalf("empty plan produced %+v", report)
		}
	}
}

func TestDocumentExample(t *testing.T) {
	var ran []string
	var blockedCalls int
	report, err := executor.Execute([]executor.Task{
		{ID: "A", Action: record("A", &ran, nil)},
		{ID: "B", Dependencies: []string{"A"}, Action: record("B", &ran, errors.New("disk full"))},
		{ID: "C", Dependencies: []string{"A"}, Action: record("C", &ran, nil)},
		{ID: "D", Dependencies: []string{"B", "C"}, Action: record("D", &ran, &blockedCalls)},
		{ID: "E", Action: record("E", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blockedCalls != 0 {
		t.Fatalf("blocked task D ran %d times", blockedCalls)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{"A", "B", "C", "E"},
		Tasks: []executor.TaskState{
			{ID: "A", Status: executor.StatusSuccess},
			{ID: "B", Status: executor.StatusFailed, Reason: "disk full"},
			{ID: "C", Status: executor.StatusSuccess},
			{ID: "D", Status: executor.StatusBlocked, BlockedBy: "B", Reason: "blocked by B (failed)"},
			{ID: "E", Status: executor.StatusSuccess},
		},
	})
}

func TestNewlyReadyTasksRunInAscendingIDOrder(t *testing.T) {
	var ran []string
	report, err := executor.Execute([]executor.Task{
		{ID: "C", Dependencies: []string{"A"}, Action: record("C", &ran, nil)},
		{ID: "B", Dependencies: []string{"A"}, Action: record("B", &ran, nil)},
		{ID: "A", Action: record("A", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{"A", "B", "C"},
		Tasks: []executor.TaskState{
			{ID: "A", Status: executor.StatusSuccess},
			{ID: "B", Status: executor.StatusSuccess},
			{ID: "C", Status: executor.StatusSuccess},
		},
	})
}

func TestStringOrderNotNumericOrder(t *testing.T) {
	var ran []string
	_, err := executor.Execute([]executor.Task{
		{ID: "task-2", Action: record("task-2", &ran, nil)},
		{ID: "task-10", Action: record("task-10", &ran, nil)},
		{ID: "m", Action: record("m", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"m", "task-10", "task-2"}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("ran %v, want %v", ran, want)
	}
}

func TestTaskWaitsForEveryDependency(t *testing.T) {
	var ran []string
	_, err := executor.Execute([]executor.Task{
		{ID: "D", Dependencies: []string{"C", "A"}, Action: record("D", &ran, nil)},
		{ID: "B", Action: record("B", &ran, nil)},
		{ID: "C", Action: record("C", &ran, nil)},
		{ID: "A", Action: record("A", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A", "B", "C", "D"}
	if !reflect.DeepEqual(ran, want) {
		t.Fatalf("ran %v, want %v", ran, want)
	}
}

func TestFailureBlocksDescendantsAndIndependentWorkContinues(t *testing.T) {
	var ran []string
	var blockedCalls int
	report, err := executor.Execute([]executor.Task{
		{ID: "A", Action: record("A", &ran, errors.New("standing is delinquent"))},
		{ID: "B", Dependencies: []string{"A"}, Action: record("B", &ran, &blockedCalls)},
		{ID: "C", Action: record("C", &ran, errors.New("gateway timeout"))},
		{ID: "D", Dependencies: []string{"B"}, Action: record("D", &ran, &blockedCalls)},
		{ID: "F", Dependencies: []string{"B", "C"}, Action: record("F", &ran, &blockedCalls)},
		{ID: "E", Action: record("E", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blockedCalls != 0 {
		t.Fatalf("blocked actions ran %d times", blockedCalls)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{"A", "C", "E"},
		Tasks: []executor.TaskState{
			{ID: "A", Status: executor.StatusFailed, Reason: "standing is delinquent"},
			{ID: "B", Status: executor.StatusBlocked, BlockedBy: "A", Reason: "blocked by A (failed)"},
			{ID: "C", Status: executor.StatusFailed, Reason: "gateway timeout"},
			{ID: "D", Status: executor.StatusBlocked, BlockedBy: "B", Reason: "blocked by B (blocked)"},
			{ID: "E", Status: executor.StatusSuccess},
			{ID: "F", Status: executor.StatusBlocked, BlockedBy: "B", Reason: "blocked by B (blocked)"},
		},
	})
}

func TestPanicIsATaskFailure(t *testing.T) {
	var ran []string
	report, err := executor.Execute([]executor.Task{
		{ID: "A", Action: func() error {
			ran = append(ran, "A")
			panic("sensor down")
		}},
		{ID: "B", Dependencies: []string{"A"}, Action: record("B", &ran, nil)},
		{ID: "C", Action: record("C", &ran, nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{"A", "C"},
		Tasks: []executor.TaskState{
			{ID: "A", Status: executor.StatusFailed, Reason: "panic: sensor down"},
			{ID: "B", Status: executor.StatusBlocked, BlockedBy: "A", Reason: "blocked by A (failed)"},
			{ID: "C", Status: executor.StatusSuccess},
		},
	})
}

func TestEachActionRunsAtMostOncePerExecution(t *testing.T) {
	var calls int
	_, err := executor.Execute([]executor.Task{
		{ID: "A", Action: func() error { calls++; return nil }},
		{ID: "B", Dependencies: []string{"A"}, Action: func() error { calls++; return nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestEachExecutionStartsFromScratch(t *testing.T) {
	var calls int
	task := func() executor.Task {
		return executor.Task{ID: "A", Action: func() error {
			calls++
			return errors.New("nope")
		}}
	}
	for i := 0; i < 2; i++ {
		report, err := executor.Execute([]executor.Task{task()})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Tasks) != 1 || report.Tasks[0].Status != executor.StatusFailed {
			t.Fatalf("run %d report %+v", i, report)
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want a fresh run each time (2)", calls)
	}
}

func TestInputOrderDoesNotChangeExecution(t *testing.T) {
	build := func(ids ...string) []executor.Task {
		tasks := make([]executor.Task, len(ids))
		for i, id := range ids {
			id := id
			tasks[i] = executor.Task{ID: id, Action: func() error { return nil }}
		}
		return tasks
	}
	first, err := executor.Execute(build("B", "C", "A"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.Execute(build("C", "A", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("order leaked into the result:\n%+v\n%+v", first, second)
	}
}

func TestCycleRejectsTheWholePlan(t *testing.T) {
	var calls int
	_, err := executor.Execute([]executor.Task{
		{ID: "Z", Action: tick(&calls)},
		{ID: "Y", Dependencies: []string{"X"}, Action: tick(&calls)},
		{ID: "X", Dependencies: []string{"Y"}, Action: tick(&calls)},
	})
	assertRejected(t, err, &calls, executor.Issue{
		Kind:    executor.IssueCycle,
		TaskID:  "X",
		Message: "X -> Y -> X",
	})
}

func TestLongerCycleIsCanonical(t *testing.T) {
	var calls int
	_, err := executor.Execute([]executor.Task{
		{ID: "B", Dependencies: []string{"A"}, Action: tick(&calls)},
		{ID: "C", Dependencies: []string{"B"}, Action: tick(&calls)},
		{ID: "A", Dependencies: []string{"C"}, Action: tick(&calls)},
	})
	assertRejected(t, err, &calls, executor.Issue{
		Kind:    executor.IssueCycle,
		TaskID:  "A",
		Message: "A -> B -> C -> A",
	})
}

func TestMissingDependencyRunsNothing(t *testing.T) {
	var calls int
	_, err := executor.Execute([]executor.Task{
		{ID: "A", Dependencies: []string{"GONE"}, Action: tick(&calls)},
		{ID: "B", Action: tick(&calls)},
	})
	assertRejected(t, err, &calls, executor.Issue{
		Kind:    executor.IssueMissingDependency,
		TaskID:  "A",
		Message: "missing dependency GONE",
	})
}

func TestSelfDependencyRunsNothing(t *testing.T) {
	var calls int
	_, err := executor.Execute([]executor.Task{
		{ID: "A", Dependencies: []string{"A"}, Action: tick(&calls)},
		{ID: "B", Action: tick(&calls)},
	})
	assertRejected(t, err, &calls, executor.Issue{
		Kind:    executor.IssueSelfDependency,
		TaskID:  "A",
		Message: "task depends on itself",
	})
}

func TestInvalidTasksRunNothing(t *testing.T) {
	var calls int
	action := tick(&calls)
	cases := []struct {
		name  string
		tasks []executor.Task
		want  executor.Issue
	}{
		{
			name:  "empty id",
			tasks: []executor.Task{{ID: "", Action: action}},
			want:  executor.Issue{Kind: executor.IssueInvalidTask, Message: "empty task id"},
		},
		{
			name: "duplicate id",
			tasks: []executor.Task{
				{ID: "A", Action: action},
				{ID: "A", Action: action},
			},
			want: executor.Issue{Kind: executor.IssueInvalidTask, TaskID: "A", Message: "duplicate task id"},
		},
		{
			name:  "nil action",
			tasks: []executor.Task{{ID: "A", Action: nil}, {ID: "B", Action: action}},
			want:  executor.Issue{Kind: executor.IssueInvalidTask, TaskID: "A", Message: "nil action"},
		},
		{
			name:  "empty dependency",
			tasks: []executor.Task{{ID: "A", Dependencies: []string{""}, Action: action}},
			want:  executor.Issue{Kind: executor.IssueInvalidTask, TaskID: "A", Message: "empty dependency id"},
		},
		{
			name:  "duplicate dependency",
			tasks: []executor.Task{{ID: "A", Dependencies: []string{"B", "B"}, Action: action}, {ID: "B", Action: action}},
			want:  executor.Issue{Kind: executor.IssueInvalidTask, TaskID: "A", Message: "duplicate dependency B"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0
			_, err := executor.Execute(tc.tasks)
			assertRejected(t, err, &calls, tc.want)
		})
	}
}

func TestEmptyFailureReasonIsStillUseful(t *testing.T) {
	report, err := executor.Execute([]executor.Task{
		{ID: "A", Action: func() error { return errors.New("") }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Tasks[0].Reason != "action failed" {
		t.Fatalf("reason %q", report.Tasks[0].Reason)
	}
}

func TestSpecPlanFile(t *testing.T) {
	src, err := os.ReadFile("examples/spec-plan.txt")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := executor.ParsePlan(string(src))
	if err != nil {
		t.Fatal(err)
	}
	report, err := executor.Execute(tasks)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{"A", "B", "C", "E"},
		Tasks: []executor.TaskState{
			{ID: "A", Status: executor.StatusSuccess},
			{ID: "B", Status: executor.StatusFailed, Reason: "disk full"},
			{ID: "C", Status: executor.StatusSuccess},
			{ID: "D", Status: executor.StatusBlocked, BlockedBy: "B", Reason: "blocked by B (failed)"},
			{ID: "E", Status: executor.StatusSuccess},
		},
	})
}

func TestParseAndRunFilingPlan(t *testing.T) {
	src, err := os.ReadFile("examples/filing-plan.txt")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := executor.ParsePlan(string(src))
	if err != nil {
		t.Fatal(err)
	}
	report, err := executor.Execute(tasks)
	if err != nil {
		t.Fatal(err)
	}
	assertReport(t, report, executor.Report{
		Executed: []string{
			"collect-records",
			"notify-stakeholders",
			"prepare-annual-report",
			"validate-standing",
		},
		Tasks: []executor.TaskState{
			{ID: "collect-records", Status: executor.StatusSuccess},
			{ID: "notify-stakeholders", Status: executor.StatusSuccess},
			{ID: "prepare-annual-report", Status: executor.StatusSuccess},
			{
				ID:        "submit-annual-report",
				Status:    executor.StatusBlocked,
				BlockedBy: "validate-standing",
				Reason:    "blocked by validate-standing (failed)",
			},
			{ID: "validate-standing", Status: executor.StatusFailed, Reason: "standing is delinquent"},
		},
	})
	text := executor.FormatReport(report)
	wantText := `executed:
- collect-records
- notify-stakeholders
- prepare-annual-report
- validate-standing
tasks:
- id: collect-records
  status: success
- id: notify-stakeholders
  status: success
- id: prepare-annual-report
  status: success
- id: submit-annual-report
  status: blocked
  blocked_by: validate-standing
  reason: blocked by validate-standing (failed)
- id: validate-standing
  status: failed
  reason: standing is delinquent
`
	if text != wantText {
		t.Fatalf("report text:\n%s\nwant:\n%s", text, wantText)
	}
}

func TestParsePlanRejectsBadLines(t *testing.T) {
	_, err := executor.ParsePlan("only-one-field\n")
	if err == nil {
		t.Fatal("expected a parse error")
	}
	_, err = executor.ParsePlan("A dance\n")
	if err == nil {
		t.Fatal("expected unknown outcome to fail")
	}
	_, err = executor.ParsePlan("A ok should not explain success\n")
	if err == nil {
		t.Fatal("expected a reason on ok to fail")
	}
}

func record(id string, ran *[]string, result any) func() error {
	return func() error {
		*ran = append(*ran, id)
		switch v := result.(type) {
		case nil:
			return nil
		case error:
			return v
		case *int:
			*v++
			return nil
		default:
			return errors.New("bad test result")
		}
	}
}

func tick(calls *int) func() error {
	return func() error {
		*calls++
		return nil
	}
}

func assertReport(t *testing.T, got, want executor.Report) {
	t.Helper()
	if !reflect.DeepEqual(got.Executed, want.Executed) {
		t.Errorf("executed:\n got %#v\nwant %#v", got.Executed, want.Executed)
	}
	if !reflect.DeepEqual(got.Tasks, want.Tasks) {
		t.Errorf("tasks:\n got %#v\nwant %#v", got.Tasks, want.Tasks)
	}
}

func assertRejected(t *testing.T, err error, calls *int, want executor.Issue) {
	t.Helper()
	if calls != nil && *calls != 0 {
		t.Errorf("actions ran %d times on a rejected plan", *calls)
	}
	var planErr *executor.PlanError
	if !errors.As(err, &planErr) {
		t.Fatalf("got %v, want *PlanError", err)
	}
	for _, issue := range planErr.Issues {
		if issue == want {
			return
		}
	}
	t.Errorf("missing issue %+v in %+v", want, planErr.Issues)
}
