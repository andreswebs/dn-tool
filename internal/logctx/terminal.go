package logctx

import (
	"io"
	"os"
)

// IsTerminalWriter reports whether w is an interactive terminal, which is what
// selects the text handler over JSON (ADR 0004). Only an *os.File can be one,
// so an injected buffer, a pipe, or a redirected file is always false: an agent,
// a supervisor, and a container runtime all get JSON without configuring
// anything.
//
// The test is the character-device bit rather than a terminal ioctl, so the
// module needs no dependency for one boolean. The difference is confined to
// character devices that are not terminals: stderr redirected to /dev/null
// reads as one, and gets text records nobody reads.
func IsTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
