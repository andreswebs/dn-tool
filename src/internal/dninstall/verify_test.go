package dninstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// verifyServer serves a binary at /dnclient and its checksum at
// /dnclient.sha256. checksumBody overrides the served checksum text; when empty
// the correct digest of binary is served. checksumStatus / binaryStatus, when
// non-zero, force that HTTP status instead of serving the body.
type verifyServer struct {
	binary         []byte
	checksumBody   string
	checksumStatus int
	binaryStatus   int
}

func (s verifyServer) start(t *testing.T) (resolved, func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/dnclient.sha256", func(w http.ResponseWriter, _ *http.Request) {
		if s.checksumStatus != 0 {
			w.WriteHeader(s.checksumStatus)
			return
		}
		body := s.checksumBody
		if body == "" {
			sum := sha256.Sum256(s.binary)
			body = hex.EncodeToString(sum[:])
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/dnclient", func(w http.ResponseWriter, _ *http.Request) {
		if s.binaryStatus != 0 {
			w.WriteHeader(s.binaryStatus)
			return
		}
		_, _ = w.Write(s.binary)
	})
	srv := httptest.NewServer(mux)
	r := resolved{
		URL:         srv.URL + "/dnclient",
		ChecksumURL: srv.URL + "/dnclient.sha256",
		Version:     "0.9.5",
	}
	return r, srv.Close
}

func TestDownloadAndVerifyMatchingChecksumSucceeds(t *testing.T) {
	binary := []byte("fake dnclient binary contents")
	r, stop := verifyServer{binary: binary}.start(t)
	defer stop()

	var dest bytes.Buffer
	if err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest); err != nil {
		t.Fatalf("downloadAndVerify: unexpected error: %v", err)
	}
	if !bytes.Equal(dest.Bytes(), binary) {
		t.Errorf("dest = %q, want %q", dest.Bytes(), binary)
	}
}

func TestDownloadAndVerifyMismatchFailsClosed(t *testing.T) {
	binary := []byte("fake dnclient binary contents")
	wrong := hex.EncodeToString(make([]byte, 32)) // 64 hex zeros, not the real digest
	r, stop := verifyServer{binary: binary, checksumBody: wrong}.start(t)
	defer stop()

	var dest bytes.Buffer
	err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest)
	if err == nil {
		t.Fatal("downloadAndVerify with wrong checksum: want error, got nil")
	}
}

func TestDownloadAndVerifyChecksumFetchFailureFailsClosed(t *testing.T) {
	binary := []byte("fake dnclient binary contents")
	r, stop := verifyServer{binary: binary, checksumStatus: http.StatusNotFound}.start(t)
	defer stop()

	var dest bytes.Buffer
	err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest)
	if err == nil {
		t.Fatal("downloadAndVerify with 404 checksum: want error, got nil")
	}
	if dest.Len() != 0 {
		t.Errorf("dest should be empty when checksum fetch fails, got %d bytes", dest.Len())
	}
}

func TestDownloadAndVerifyBinaryFetchFailureFails(t *testing.T) {
	binary := []byte("fake dnclient binary contents")
	r, stop := verifyServer{binary: binary, binaryStatus: http.StatusInternalServerError}.start(t)
	defer stop()

	var dest bytes.Buffer
	err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest)
	if err == nil {
		t.Fatal("downloadAndVerify with 500 binary: want error, got nil")
	}
}

func TestDownloadAndVerifyDigestOverActualStream(t *testing.T) {
	// Server advertises the checksum of the full binary but serves a truncated
	// body, so the digest computed over the actual stream must not match.
	full := []byte("the complete and correct dnclient binary")
	sum := sha256.Sum256(full)
	r, stop := verifyServer{binary: []byte("truncated"), checksumBody: hex.EncodeToString(sum[:])}.start(t)
	defer stop()

	var dest bytes.Buffer
	err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest)
	if err == nil {
		t.Fatal("downloadAndVerify over truncated stream: want error, got nil")
	}
}

func TestDownloadAndVerifyToleratesChecksumWhitespace(t *testing.T) {
	binary := []byte("fake dnclient binary contents")
	sum := sha256.Sum256(binary)
	r, stop := verifyServer{binary: binary, checksumBody: "  " + hex.EncodeToString(sum[:]) + "\n"}.start(t)
	defer stop()

	var dest bytes.Buffer
	if err := downloadAndVerify(context.Background(), http.DefaultClient, r, &dest); err != nil {
		t.Fatalf("downloadAndVerify with padded checksum: unexpected error: %v", err)
	}
}
