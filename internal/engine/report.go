package engine

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/noi/dwbt/internal/runner"
)

// WriteSteps writes the status of each step of res, with the error of the
// step that did not pass, and the error outside the steps, if any.
func WriteSteps(w io.Writer, res *runner.Result) {
	for i, s := range res.Steps {
		writeBindingErr(w, res, i)
		label := s.Step.Label()
		if s.Section != runner.Main {
			label = s.Section.String() + ": " + label
		}
		if s.Status == runner.Skipped {
			fmt.Fprintf(w, "  %-5s %s\n", s.Status, label)
			continue
		}
		fmt.Fprintf(w, "  %-5s %s  %s\n", s.Status, label, Duration(s.Duration))
		writeErr(w, s.Err)
	}
	writeBindingErr(w, res, len(res.Steps))
}

// writeBindingErr writes the error outside the steps of res if it occurred
// after the first n steps.
func writeBindingErr(w io.Writer, res *runner.Result, n int) {
	if res.Err != nil && res.Err.At == n {
		fmt.Fprintf(w, "  %-5s %s\n", runner.Errored, res.Err.In)
		writeErr(w, res.Err.Err)
	}
}

func writeErr(w io.Writer, err error) {
	if err == nil {
		return
	}
	for _, line := range strings.Split(err.Error(), "\n") {
		fmt.Fprintf(w, "        %s\n", line)
	}
}

// Duration formats d for reports.
func Duration(d time.Duration) string {
	if d < time.Millisecond {
		return d.Round(time.Microsecond).String()
	}
	return d.Round(time.Millisecond).String()
}
