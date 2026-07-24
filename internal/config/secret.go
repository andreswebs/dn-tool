package config

import "log/slog"

// redactedMarker is the placeholder rendered in place of a Secret's value
// across every logging and serialization path.
const redactedMarker = "REDACTED"

// Secret is a string whose value is structurally kept out of logs and serialized
// output. It renders as REDACTED under slog (LogValue), fmt (String), and
// encoding/json (MarshalJSON), so accidentally logging or marshaling a struct
// that embeds it — e.g. Config or an API response — can never leak the value.
// The raw value is reachable only through Reveal, an explicit, greppable call
// confined to the few sites that genuinely need it (the auth header, the
// env-file writer). Closes upstream SEC5.
type Secret string

// LogValue redacts the secret when logged through slog.
func (Secret) LogValue() slog.Value { return slog.StringValue(redactedMarker) }

// String redacts the secret under fmt's %v/%s/%q verbs.
func (Secret) String() string { return redactedMarker }

// GoString redacts the secret under fmt's %#v verb, which would otherwise print
// the underlying value of a defined string type.
func (Secret) GoString() string { return redactedMarker }

// MarshalJSON redacts the secret when a containing struct is JSON-encoded,
// including by the slog JSON handler.
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redactedMarker + `"`), nil }

// Reveal returns the underlying secret value. It is the only way to obtain the
// raw string and must be called only where the value is actually required.
func (s Secret) Reveal() string { return string(s) }
