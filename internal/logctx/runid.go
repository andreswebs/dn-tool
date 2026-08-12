package logctx

import (
	"crypto/rand"
	"strings"
)

// runIDLen is the width of a run id: 16 characters over crypto/rand.Text's
// base32 alphabet is 80 bits, far more than enough to separate concurrent
// invocations in a shared log stream, and short enough not to bloat every
// record. A fixed width is what lets a golden test normalize it with one
// substitution.
const runIDLen = 16

// NewRunID returns a random lowercase id correlating every record of one
// invocation. It is sourced from crypto/rand.Text, which cannot fail; the
// alternative, hex over rand.Read, would force an unreachable error branch into
// a function that has no way to degrade.
func NewRunID() string {
	return strings.ToLower(rand.Text()[:runIDLen])
}
