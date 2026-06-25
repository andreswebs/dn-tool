# defined.net API & Downloads — Reference

Reference for the subset of the [defined.net](https://defined.net) REST API and
the `dnclient` download infrastructure that `dn-tool` consumes. It describes what
the API returns; it does not prescribe how to call it. For the tool's design see
[dn-tool-design.md](../dn-tool-design.md).

**Provenance.** Compiled 2026-06-06 from the official OpenAPI 3.1 spec at
<https://docs.defined.net/openapi.yaml> (the source behind the rendered docs at
<https://docs.defined.net/api/>) and from live responses of
`https://api.defined.net/v1/downloads` and `https://dl.defined.net`. Latest
`dnclient` observed: **v0.9.5**. The API has no version/date header; treat
version-specific facts (latest version, endpoint deprecations) as point-in-time.

---

## 1. Pending questions answered

| Question                                                                    | Answer                                                                                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Can the binary's SHA-256 be obtained for verification?                      | **Yes.** A sibling file `<binary-url>.sha256` is served next to every download (HTTP 200). Its body is a bare 64-character lowercase hex digest of the binary. Verified: the downloaded `linux/amd64` v0.9.5 binary hashes to exactly the value in its `.sha256`. See [§6](#6-binary-download--checksum-verification).                                              |
| Does the `/v1/downloads` **API** advertise a checksum?                      | **No.** The JSON contains only download URLs. The checksum lives at the sibling `.sha256` URL on `dl.defined.net`, not in the API response.                                                                                                                                                                                                                         |
| How do we detect an existing remote host by name (the enroll orphan check)? | There is **no `name` filter** on the host-list endpoints. Two options: (a) attempt `POST /v2/host-and-enrollment-code` and treat a `400 ERR_DUPLICATE_VALUE` on `path: name` as "remote record present"; or (b) paginate `GET /v2/hosts?filter.networkID=…` and match `name` client-side. See [§4.2](#42-list-hosts) and [§7](#7-implementation-notes-for-dn-tool). |
| Which create/delete endpoints are current?                                  | Use **`POST /v2/host-and-enrollment-code`** (v1 is deprecated). Host **delete is `DELETE /v1/hosts/{hostID}`** and is **not** deprecated. See [§3](#3-api-versioning).                                                                                                                                                                                              |

---

## 2. Conventions

### 2.1 Base URL and authentication

- **Base URL:** `https://api.defined.net` (single documented server).
- **Auth:** API key as a bearer token — `Authorization: Bearer <DN_API_KEY>`.
  Keys are issued from <https://admin.defined.net/settings/api-keys> and carry
  explicit permission scopes. The `/v1/downloads` endpoint is the only
  unauthenticated operation listed.
- **Content type:** `application/json` for request and response bodies.

### 2.2 Response envelope

Successful responses wrap the payload in a `data` field alongside a `metadata`
object (the latter is `{}` for single-resource responses and holds pagination
info for list responses):

```json
{
  "data": {
    /* resource or array */
  },
  "metadata": {}
}
```

### 2.3 Error envelope

Error responses (e.g. `400`) carry an `errors` array. Each error has a static
`code`, a human `message`, and an optional `path` naming the offending field:

```json
{
  "errors": [
    {
      "code": "ERR_DUPLICATE_VALUE",
      "message": "value already exists",
      "path": "name"
    }
  ]
}
```

Error codes observed on host endpoints:

| Code                              | Meaning                                            | `path`                                   |
| --------------------------------- | -------------------------------------------------- | ---------------------------------------- |
| `ERR_DUPLICATE_VALUE`             | A host with that name already exists               | `name`                                   |
| `ERR_INVALID_REFERENCE`           | Referenced ID does not exist                       | `networkID` (e.g.)                       |
| `ERR_INVALID_VALUE`               | Constraint violated (lighthouse/relay rules)       | `staticAddresses`, `listenPort`, or none |
| `ERR_CERT_VERSION_2_INCOMPATIBLE` | Used a v1 endpoint on a non-IPv4-only (v2) network | —                                        |

### 2.4 Pagination

List endpoints are cursor-paginated via query parameters:

| Parameter       | Type    | Default | Notes                                                                 |
| --------------- | ------- | ------- | --------------------------------------------------------------------- |
| `pageSize`      | integer | 25      | Max 500.                                                              |
| `cursor`        | string  | —       | Opaque; from `nextCursor`/`prevCursor` of a prior response.           |
| `includeCounts` | boolean | false   | When true, `metadata` includes `totalCount` and `page.{start,count}`. |

Response `metadata` (`PaginationMetadata`): `hasNextPage`, `hasPrevPage`,
`nextCursor`, `prevCursor`, optional `totalCount`, optional `page.{count,start}`.

---

## 3. API versioning

The API mixes versions per operation. Relevant to `dn-tool`:

| Operation                     | Current endpoint                    | Deprecated form                     |
| ----------------------------- | ----------------------------------- | ----------------------------------- |
| Create host + enrollment code | `POST /v2/host-and-enrollment-code` | `POST /v1/host-and-enrollment-code` |
| Create host (no code)         | `POST /v2/hosts`                    | `POST /v1/hosts`                    |
| Get host                      | `GET /v2/hosts/{hostID}`            | `GET /v1/hosts/{hostID}`            |
| List hosts                    | `GET /v2/hosts`                     | `GET /v1/hosts`                     |
| Edit host                     | `PUT /v3/hosts/{hostID}`            | `PUT /v2/hosts/{hostID}`            |
| **Delete host**               | **`DELETE /v1/hosts/{hostID}`**     | _(not deprecated; no v2/v3 form)_   |

**Network cert version.** v1 host endpoints serve **IPv4-only** networks. Calling
a v1 endpoint against a v2 (IPv4/IPv6) network returns `400`
`ERR_CERT_VERSION_2_INCOMPATIBLE`. The v2/v3 host endpoints return `ipAddresses`
(an array, up to one IPv4 + one IPv6); the deprecated v1 shape used a single
`ipAddress` string. `dn-tool` should prefer the v2/v3 endpoints, which work on
both network types.

---

## 4. Endpoints used by dn-tool

### 4.1 Create host & enrollment code

```txt
POST /v2/host-and-enrollment-code
Token scopes: hosts:create, hosts:enroll
```

Creates the remote host record **and** a single-use enrollment code in one call.

Request body:

| Field                 | Type            | Required           | Notes                                                                                                                     |
| --------------------- | --------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| `name`                | string (1–255)  | yes                | Host display name; must be unique in the network.                                                                         |
| `networkID`           | string          | yes                | `network-…`.                                                                                                              |
| `roleID`              | string \| null  | no                 | `role-…`.                                                                                                                 |
| `ipAddresses`         | string[] (≤2)   | no                 | IPs to assign and/or CIDR prefixes to auto-assign within. ≤1 IPv4 + ≤1 IPv6. Auto-assigned if omitted (per network type). |
| `staticAddresses`     | string[]        | no                 | `IPv4:port` / `hostname:port`. **≥1 required if `isLighthouse`.**                                                         |
| `listenPort`          | integer 0–65535 | no (default 0)     | 0 = auto-select. **Non-zero required for lighthouse/relay.**                                                              |
| `isLighthouse`        | boolean         | no                 | Mutually exclusive with `isRelay`.                                                                                        |
| `isRelay`             | boolean         | no                 | Mutually exclusive with `isLighthouse`.                                                                                   |
| `tags`                | string[]        | no                 | `key:value` (key ≤20, value ≤50, no surrounding whitespace).                                                              |
| `configOverrides`     | array           | no                 | Nebula config overrides.                                                                                                  |
| `codeLifetimeSeconds` | int64           | no (default 86400) | Enrollment-code lifetime (24h default).                                                                                   |

> Note: the design's `DN_IP_ADDRESS` maps to a single entry in `ipAddresses` on
> the v2 endpoint (the deprecated v1 endpoint used a scalar `ipAddress`).

Success `200` — `data.host` is a [Host](#5-host-object) and `data.enrollmentCode`
holds the code:

```json
{
  "data": {
    "host": {
      "id": "host-24NV…",
      "name": "My new host",
      "networkID": "network-…",
      "...": "..."
    },
    "enrollmentCode": {
      "code": "H8NEbm99QvupjqW1PsdVR9DNSiFmoQtJXyGTQxerlSU",
      "lifetimeSeconds": 86400
    }
  },
  "metadata": {}
}
```

The `code` is what is handed to `dnclient enroll -code <code>`. It is sensitive
and single-use — never log it.

Documented `400` cases (all in the `errors` array):

| Summary                        | `code`                  | `path`            |
| ------------------------------ | ----------------------- | ----------------- |
| Host name already exists       | `ERR_DUPLICATE_VALUE`   | `name`            |
| Unknown `networkID`            | `ERR_INVALID_REFERENCE` | `networkID`       |
| Lighthouse AND relay           | `ERR_INVALID_VALUE`     | —                 |
| Lighthouse without static IP   | `ERR_INVALID_VALUE`     | `staticAddresses` |
| Lighthouse without static port | `ERR_INVALID_VALUE`     | `listenPort`      |
| Relay without static port      | `ERR_INVALID_VALUE`     | `listenPort`      |

> The API enforces the lighthouse/relay rules server-side; `dn-tool` validates
> them locally too (design Req 3) to fail fast without a network round-trip.

### 4.2 List hosts

```txt
GET /v2/hosts
Token scope: hosts:list
```

Returns a paginated array of [Host](#5-host-object) objects. Available filters
(query params): `filter.networkID`, `filter.roleID`, `filter.isBlocked`,
`filter.isLighthouse`, `filter.isRelay`, `filter.tag`,
`filter.metadata.{lastSeenAt,platform,updateAvailable}`,
`filter.endpointOIDCUserID` — plus the pagination params from [§2.4](#24-pagination).

**There is no `name` filter.** To find a host by name, filter by `networkID` and
match `name` client-side across pages, or detect the duplicate via the create
call ([§7](#7-implementation-notes-for-dn-tool)).

### 4.3 Get host

```txt
GET /v2/hosts/{hostID}
Token scope: hosts:read
```

`200` → `{ data: Host, metadata: {} }`. `404` if the host does not exist.

### 4.4 Delete host

```txt
DELETE /v1/hosts/{hostID}
Token scope: hosts:delete
```

`200` → `{ "data": {}, "metadata": {} }`. Used by `unenroll`. The spec documents
only the `200` body; a missing record is expected to surface as `404`, which
`dn-tool` treats as idempotent success per the unenroll invariant (design §2.5).
This is the only host endpoint with no v2/v3 successor.

### 4.5 List software downloads

```txt
GET /v1/downloads          (unauthenticated)
Token scope: none
```

Returns the [Downloads](#6-binary-download--checksum-verification) object. Used by
`install` to resolve a download URL for the host's OS/arch and version.

---

## 5. Host object

The `HostV2` schema (returned by v2/v3 host endpoints and inside the
enrollment-code response):

| Field                                    | Type            | Notes                                                                                        |
| ---------------------------------------- | --------------- | -------------------------------------------------------------------------------------------- |
| `id`                                     | string          | `host-…` — the remote record identifier.                                                     |
| `organizationID`                         | string          | `org-…`.                                                                                     |
| `networkID`                              | string          | `network-…`.                                                                                 |
| `roleID`                                 | string \| null  | `role-…`.                                                                                    |
| `endpointOIDCUserID`                     | string \| null  |                                                                                              |
| `name`                                   | string          | Display name.                                                                                |
| `ipAddresses`                            | string[]        | Assigned overlay IPs (IPv4 and/or IPv6).                                                     |
| `staticAddresses`                        | string[]        | `address:port`.                                                                              |
| `listenPort`                             | int32 (0–65535) | 0 for a regular host.                                                                        |
| `isLighthouse` / `isRelay` / `isBlocked` | boolean         |                                                                                              |
| `createdAt` / `modifiedAt`               | date-time       | RFC 3339.                                                                                    |
| `tags`                                   | string[]        | `key:value`.                                                                                 |
| `configOverrides`                        | array           |                                                                                              |
| `metadata`                               | object          | `{ lastSeenAt, platform, updateAvailable, version }` — all null until the host has enrolled. |

> `host.id` is the value `unenroll` needs. On an enrolled host it is also written
> by `dnclient` into `<config-root>/<network>/dnclient.yml` at `metadata.host_id`
> (the field `dn-tool` parses — design §2.6). The current `dnclient` (0.9.5)
> writes under `/var/lib/defined`, not the `/etc/defined` older docs assume, and
> nests `host_id` under `metadata` (and a copy under `host_key`), not at the root.
> That file's schema and location are owned by `dnclient`, not this API.

---

## 6. Binary download & checksum verification

### 6.1 Downloads object (`GET /v1/downloads`)

```json
{
  "data": {
    "dnclient": {
      "0.9.5": {
        "linux-amd64": "https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient",
        "...": "..."
      },
      "latest": {
        "linux-amd64": "https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient",
        "...": "..."
      }
    },
    "mobile": { "android": "…", "ios": "…" },
    "container": { "docker": "https://hub.docker.com/r/definednet/dnclient/" },
    "versionInfo": {
      "dnclient": { "0.9.5": { "latest": true, "releaseDate": "2025-…" } },
      "latest": { "dnclient": "0.9.5", "mobile": "0.5.1" }
    }
  }
}
```

Shape: `data.dnclient.<version>.<os-arch> = <download-url>`. Resolving "latest":
the map has a literal `"latest"` key (mirrors the newest version), and
`data.versionInfo.latest.dnclient` gives the latest version **string** (e.g.
`"0.9.5"`) if a concrete version is preferred over the alias.

> The upstream `install` script selects with
> `jq '.data.dnclient | .[$version] | .[$osarch]'`. The deprecated upstream
> `DN_VERSION` values were bare (`0.3.2`); current keys are also bare
> (`0.9.5`, not `v0.9.5`), while the URL path segment is `v0.9.5`.

### 6.2 OS/architecture keys

Keys are `<os>-<arch>`. Linux keys (the only ones relevant — `dn-tool` fails on
non-Linux per design Req 1):

| Host arch (`uname -m` / `GOARCH`) | Downloads key | URL path segment |
| --------------------------------- | ------------- | ---------------- |
| `x86_64` / `amd64`                | `linux-amd64` | `linux/amd64`    |
| `aarch64` / `arm64`               | `linux-arm64` | `linux/arm64`    |
| `i386`,`i686`,`x86` / `386`       | `linux-386`   | `linux/386`      |
| `armv5*` / `arm`+GOARM=5          | `linux-armv5` | `linux/arm-5`    |
| `armv6*` / `arm`+GOARM=6          | `linux-armv6` | `linux/arm-6`    |
| `armv7*` / `arm`+GOARM=7          | `linux-armv7` | `linux/arm-7`    |

Also published for Linux: `linux-mips`, `linux-mips-softfloat`, `linux-mips64`,
`linux-mips64le`, `linux-mipsle`, `linux-ppc64le`, `linux-riscv64`. (Non-Linux
keys: `freebsd-amd64/arm64`, `macos-universal-server`/`-server-dmg`/`-desktop`,
`windows-amd64/arm64-server`/`-desktop`.) Note the key uses `armv5/6/7` but the
URL path uses `arm-5/6/7`.

The Linux server binary is a **statically-linked ELF** (verified on
`linux/amd64`), matching the design's static-binary deployment model.

### 6.3 Checksum

For any binary URL, appending `.sha256` yields its checksum file:

```txt
https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient
https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient.sha256   →  3d506fbadee236d34f75219ff02e475299496704fc9d4dacfdc7f55d612292f5
```

- Body: a single lowercase 64-char hex SHA-256, no filename or trailing data.
- Confirmed present for multiple arches (`linux/amd64`, `linux/arm64`, `macos`)
  and older versions (`v0.9.1`).
- **Verified** to equal the SHA-256 of the corresponding binary.
- `dl.defined.net` is S3-backed behind CloudFront (the binary response carries an
  MD5-style `etag` and `x-amz-*` headers); the `etag` is **not** a usable SHA-256.

This is the published checksum source the design's Requirement 1 refers to: fetch
`<url>.sha256` and verify before installing. Verification is mandatory — if the
checksum cannot be fetched or does not match, the binary is not installed.

---

## 7. Implementation notes for dn-tool

Descriptive mapping of the above to the tool's behavior (see the design doc for
the authoritative requirements):

- **Token scopes** the API key must hold: `hosts:create` + `hosts:enroll` (enroll),
  `hosts:delete` (unenroll), and `hosts:read`/`hosts:list` if the orphan check
  uses GET rather than the create-duplicate signal. `/v1/downloads` needs none.
- **Endpoint selection:** create via `POST /v2/host-and-enrollment-code`; delete
  via `DELETE /v1/hosts/{hostID}`; read/list via the v2 host endpoints. Avoid the
  deprecated v1 create/get (and the `ipAddress` scalar shape).
- **Orphan detection (enroll state machine, design §2.4):** with no `name` filter,
  "remote record present, local absent" is detectable either by attempting the
  create and matching `400 ERR_DUPLICATE_VALUE`/`path: name`, or by paging
  `GET /v2/hosts?filter.networkID=…` and matching `name`. The create-duplicate
  signal is one round-trip and needs no `hosts:list` scope, but does not return
  the existing `host.id` (needed for the `--force` delete path) — that still
  requires a list/match. The list approach yields the `id` directly.
- **Retry classification (design Req 9):** retry transient failures (network, 5xx,
  429); never retry `400` validation errors — they carry actionable `errors[]`
  with `code`/`path`. Verify HTTP status before reading the body.
- **404 on delete** is idempotent success for `unenroll` (design §2.5 invariant).
- **Secrets:** the enrollment `code` and the `DN_API_KEY` bearer token must never
  be logged (design Req 7 / upstream SEC5).
- **Checksum:** the API does not embed checksums; fetch `<binary-url>.sha256` and
  verify (mandatory — there is no configured-checksum override).

---

## Sources

- defined.net OpenAPI 3.1 spec: <https://docs.defined.net/openapi.yaml>
- Rendered API docs: <https://docs.defined.net/api/>
- Automating host creation guide: <https://docs.defined.net/guides/automating-host-creation/>
- Live downloads endpoint: `https://api.defined.net/v1/downloads`
- Binary + checksum host: `https://dl.defined.net/<hash>/v<version>/<os>/<arch>/dnclient[.sha256]`
- API keys / scopes: <https://admin.defined.net/settings/api-keys>
