package output

import (
	"errors"
	"io"

	"github.com/andreswebs/dn-tool/internal/terr"
)

// codeUnclassified is the machine code an error without a classification renders
// with. It pairs with CodeInternal (70): an unclassified error is a bug, and
// naming it as one keeps the gap visible rather than mislabeling the failure as a
// class it does not belong to.
const codeUnclassified = "internal_error"

// errorEnvelope is the failure shape on stderr (ADR 0005): the same head every
// result carries, ok always false, and the typed error's public projection.
type errorEnvelope struct {
	Head
	Error errorDetail `json:"error"`
}

// errorDetail is the error object consumers branch on: code is the stable
// snake_case token, message the human-readable text, and hint and details are
// present only when the error carries them.
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Details any    `json:"details,omitempty"`
}

// EmitError renders err as exactly one newline-terminated JSON error envelope on
// w (stderr in production), and writes nothing on a nil error. The code and hint
// come from the error's terr.Coded classification and the details from
// terr.Detailed, both discovered through the chain, so context added on the way
// out never loses them.
//
// The write is best-effort and returns nothing: a failure to report a failure is
// unrecoverable, and escalating it would displace the error it describes. An
// unmarshalable details payload degrades to the same envelope without details,
// which cannot fail to marshal.
func EmitError(w io.Writer, err error) {
	if err == nil {
		return
	}

	env := errorEnvelope{
		Head:  Head{SchemaVersion: SchemaVersion},
		Error: errorDetail{Code: codeUnclassified, Message: err.Error()},
	}
	var coded terr.Coded
	if errors.As(err, &coded) {
		env.Error.Code = coded.Code()
		env.Error.Hint = coded.Hint()
	}
	var detailed terr.Detailed
	if errors.As(err, &detailed) {
		env.Error.Details = detailed.ErrorDetails()
	}

	data, marshalErr := marshalLine(env)
	if marshalErr != nil {
		env.Error.Details = nil
		if data, marshalErr = marshalLine(env); marshalErr != nil {
			return
		}
	}
	// Sanctioned blank assignment (ADR 0002): the diagnostic sink failing cannot
	// change the exit code, and there is no second channel to report it on.
	_, _ = w.Write(data)
}
