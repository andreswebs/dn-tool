package api

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrAPIRejected is a 4xx from the management API: the request reached it and it
// refused the payload or arguments. It carries EX_DATAERR (65) because the data
// dn-tool sent (or the identifiers it was configured with) is what the remote
// side rejected — retrying unchanged cannot succeed. The typed *APIError stays in
// the chain, so callers still branch on its static code/path.
var ErrAPIRejected = terr.New("api_rejected", output.CodeData,
	"check the DN_* identifiers and the request the message names against the defined.net API reference",
	"the management API rejected the request")

// ErrAPIUnavailable is a transport failure (DNS, connection, retry-exhausted
// timeout) or a 5xx. It carries EX_UNAVAILABLE (69), the sysexits class for a
// dependency that could not serve the request: dn-tool's inputs may be perfectly
// valid, so this is the one API class where a later retry is the right response.
var ErrAPIUnavailable = terr.New("api_unavailable", output.CodeUnavailable,
	"check network reachability of DN_API_URL and retry; the failure is on the API side or in transit",
	"the management API was unreachable")

// ErrAPIMalformedResponse is a 2xx whose body could not be read or does not match
// the documented envelope. It carries EX_DATAERR (65) rather than EX_SOFTWARE
// (70): the data received is bad, which is not dn-tool's own bug — 70 is reserved
// for faults on dn-tool's side of the exchange, such as building the request.
var ErrAPIMalformedResponse = terr.New("api_malformed_response", output.CodeData,
	"the API returned a body dn-tool cannot decode; verify DN_API_URL addresses the defined.net API and retry",
	"malformed management API response")
