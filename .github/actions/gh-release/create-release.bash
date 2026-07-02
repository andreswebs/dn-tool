#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

: "${TAG:?TAG is required}"
: "${REPO:?REPO is required}"
DIST_DIR="${DIST_DIR:-dist}"

prerelease=()
# Pre-release tags carry a suffix after a hyphen (e.g. v1.2.0-rc1).
case "${TAG}" in
*-*) prerelease=(--prerelease) ;;
esac

# The notes template stays a literal heredoc so its backtick code fences are not
# treated as command substitution; @REPO@ is substituted afterwards.
notes="$(
    cat <<'EOF'
## Verifying this release

Artifacts are signed with Sigstore keyless signing and carry SLSA build
provenance. Verification needs cosign v3+ and gh v2.49+.

### Checksums signature (cosign)

```sh
cosign verify-blob \
  --bundle SHA256SUMS.txt.sigstore.json \
  --certificate-identity-regexp "https://github.com/@REPO@/.github/workflows/release.yml@refs/tags/.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt
```

### Build provenance (GitHub attestations)

```sh
gh attestation verify dn-tool-linux-amd64-*.tar.gz --repo @REPO@
```

### SBOM signature (cosign)

```sh
cosign verify-blob \
  --bundle dn-tool-*.spdx.json.sigstore.json \
  --certificate-identity-regexp "https://github.com/@REPO@/.github/workflows/release.yml@refs/tags/.*" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  dn-tool-*.spdx.json
```
EOF
)"
notes="${notes//@REPO@/${REPO}}"

gh release create "${TAG}" \
    --title "${TAG}" \
    --notes "${notes}" \
    --generate-notes \
    "${prerelease[@]}" \
    "${DIST_DIR}"/*.tar.gz \
    "${DIST_DIR}/SHA256SUMS.txt" \
    "${DIST_DIR}/SHA256SUMS.txt.sigstore.json" \
    "${DIST_DIR}/dn-tool-${TAG}.spdx.json" \
    "${DIST_DIR}/dn-tool-${TAG}.spdx.json.sigstore.json"
