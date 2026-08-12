package dnstate

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrNotEnrolled reports that no local dnclient configuration exists for the
// network, so the host does not appear to be enrolled. unenroll matches it with
// errors.Is to distinguish a not-enrolled host from a genuine read failure.
//
// It carries EX_IOERR (74) because it reaches the user through unenroll's local
// state read, which is that class: keeping 74 means the code is a pure
// classification of behavior that already existed. Recorded reservation: "unenroll
// a host that was never enrolled" reads oddly as an I/O error, but re-classifying
// it is a behavior change outside this epic's scope.
var ErrNotEnrolled = terr.New("not_enrolled", output.CodeIO,
	"no local dnclient config exists for this network; the host is not enrolled, so there is nothing to unenroll",
	"host does not appear to be enrolled")
