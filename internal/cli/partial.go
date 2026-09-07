package cli

import (
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// printedError marks an error whose data envelope was already written to
// stdout. Run maps it to an exit code without printing a second envelope,
// which is how partial success keeps its data and still exits 6.
type printedError struct {
	err error
}

func (e *printedError) Error() string { return e.err.Error() }

func (e *printedError) Unwrap() error { return e.err }

// PrintPartial prints the success envelope with the data intact and
// returns an error that makes the process exit with the code of err
// (ExitPartial for CodePartial) without a second envelope on stdout.
func (c *Context) PrintPartial(env *output.Envelope, err *figctl.Error) error {
	if perr := c.Printer.Print(env); perr != nil {
		return perr
	}
	return &printedError{err: err}
}
