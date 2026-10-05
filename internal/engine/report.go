package engine

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/noi/dwbt/internal/runner"
)

// WriteSteps writes the status of each step of res, with the error of the
// step that did not pass.
func WriteSteps(w io.Writer, res *runner.Result) {
	for _, s := range res.Steps {
		if s.Status == runner.Skipped {
			fmt.Fprintf(w, "  %-5s %s\n", s.Status, s.Step.Label())
			continue
		}
		fmt.Fprintf(w, "  %-5s %s  %s\n", s.Status, s.Step.Label(), Duration(s.Duration))
		if s.Err != nil {
			for _, line := range strings.Split(s.Err.Error(), "\n") {
				fmt.Fprintf(w, "        %s\n", line)
			}
		}
	}
}

// Duration formats d for reports.
func Duration(d time.Duration) string {
	if d < time.Millisecond {
		return d.Round(time.Microsecond).String()
	}
	return d.Round(time.Millisecond).String()
}
