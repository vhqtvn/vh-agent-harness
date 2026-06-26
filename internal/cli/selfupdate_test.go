package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func makeArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	want := []byte("\x7fELF-pretend-binary")
	archive := makeArchive(t, map[string][]byte{
		"LICENSE":          []byte("license text"),
		"README.md":        []byte("readme"),
		"vh-agent-harness": want,
	})

	got, err := extractBinary(archive, "vh-agent-harness")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted bytes mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestExtractBinaryMissing(t *testing.T) {
	archive := makeArchive(t, map[string][]byte{"README.md": []byte("readme")})
	if _, err := extractBinary(archive, "vh-agent-harness"); err == nil {
		t.Fatal("expected error for archive without the harness binary, got nil")
	}
}

func TestExtractBinaryNotGzip(t *testing.T) {
	if _, err := extractBinary([]byte("not a gzip stream"), "vh-agent-harness"); err == nil {
		t.Fatal("expected error for non-gzip input, got nil")
	}
}

func TestParseChecksums(t *testing.T) {
	body := "" +
		"abc123  vh-agent-harness_0.1.0_linux_amd64.tar.gz\n" +
		"def456  vh-agent-harness_0.1.0_darwin_arm64.tar.gz\n" +
		"deadbeef *vh-agent-harness_0.1.0_linux_arm64.tar.gz\n" // binary-mode "*" prefix

	cases := []struct {
		name string
		want string
	}{
		{"vh-agent-harness_0.1.0_linux_amd64.tar.gz", "abc123"},
		{"vh-agent-harness_0.1.0_darwin_arm64.tar.gz", "def456"},
		{"vh-agent-harness_0.1.0_linux_arm64.tar.gz", "deadbeef"}, // "*" stripped
		{"vh-agent-harness_0.1.0_windows_amd64.tar.gz", ""},       // absent
	}
	for _, c := range cases {
		if got := parseChecksums(body, c.name); got != c.want {
			t.Errorf("parseChecksums(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
