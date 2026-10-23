package main

import (
	"fmt"
	"os"

	"github.com/felipe-medeiros-alves/task-executor"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: taskexecutor <plan.txt>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read plan: %v\n", err)
		os.Exit(1)
	}
	tasks, err := executor.ParsePlan(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse plan: %v\n", err)
		os.Exit(1)
	}
	report, err := executor.Execute(tasks)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	fmt.Print(executor.FormatReport(report))
}
