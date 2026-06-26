package cli

// self-update downloads the latest release archive for this OS/arch, verifies
// it against the published checksums.txt, extracts the `harness` binary, and
// atomically replaces the running executable.
//
// This is the BINARY self-updater. It is deliberately a distinct verb from
// `update`, which re-renders and reconciles the harness *inside a project* (the
// seam apply). `self-update` never touches a project tree — it only swaps the
// binary on disk.
//
// goreleaser publishes `vh-agent-harness_<version>_<os>_<arch>.tar.gz` archives
// plus a `checksums.txt`, so we verify the *archive* digest, then unpack the
// inner `vh-agent-harness` binary.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// defaultRepoAPI is the upstream GitHub repo (API base). Overridable with --repo.
const defaultRepoAPI = "https://api.github.com/repos/vhqtvn/vh-agent-harness"

var (
	selfUpdateRepo  string
	selfUpdateForce bool
	selfUpdateYes   bool
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Download and install the latest harness binary (verified by checksums.txt)",
	Long: `Check the latest GitHub release, download the release archive for this
OS/arch, verify its SHA256 against the published checksums.txt, unpack the
harness binary, and atomically replace the running executable.

This updates the BINARY only. It does not touch any project install — use
'vh-agent-harness update' to re-render and reconcile a harness inside a project.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runSelfUpdate(cmd.OutOrStdout())
	},
}

func init() {
	selfUpdateCmd.Flags().StringVar(&selfUpdateRepo, "repo", defaultRepoAPI, "GitHub repo API base URL")
	selfUpdateCmd.Flags().BoolVar(&selfUpdateForce, "force", false, "reinstall even if already on the latest version")
	selfUpdateCmd.Flags().BoolVar(&selfUpdateYes, "yes", false, "skip the confirmation prompt")
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func runSelfUpdate(out io.Writer) error {
	client := &http.Client{Timeout: 60 * time.Second}

	rel, err := latestRelease(client, selfUpdateRepo)
	if err != nil {
		return err
	}

	// goreleaser injects {{.Version}} (tag without the leading "v") into
	// cli.Version, while the release tag keeps the "v". Normalise both.
	current := strings.TrimPrefix(Version, "v")
	latest := strings.TrimPrefix(rel.TagName, "v")
	fmt.Fprintf(out, "Current: %s   Latest: %s\n", Version, rel.TagName)
	if current == latest && !selfUpdateForce {
		fmt.Fprintln(out, "Already up to date.")
		return nil
	}

	// Archive name matches .goreleaser.yml name_template:
	//   {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}  (+ .tar.gz)
	archiveName := fmt.Sprintf("vh-agent-harness_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	asset := findAsset(rel.Assets, archiveName)
	if asset == nil {
		return fmt.Errorf("no release asset %q for this platform in %s", archiveName, rel.TagName)
	}

	if !selfUpdateYes {
		fmt.Fprintf(out, "Install %s (%s)?\n", rel.TagName, archiveName)
		fmt.Fprint(out, "Proceed? [y/N]: ")
		var ans string
		_, _ = fmt.Scanln(&ans)
		if !strings.EqualFold(strings.TrimSpace(ans), "y") {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	// Expected checksum for the ARCHIVE from the release's checksums.txt.
	want, err := expectedSum(client, rel.Assets, archiveName)
	if err != nil {
		return err
	}
	if want == "" {
		return fmt.Errorf("no checksums.txt entry for %s; refusing to install unverified binary", archiveName)
	}

	fmt.Fprintf(out, "Downloading %s…\n", archiveName)
	data, err := download(client, asset.URL)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", got, want)
	}

	bin, err := extractBinary(data, "vh-agent-harness")
	if err != nil {
		return err
	}

	target, err := replaceSelf(bin, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s to %s.\n", rel.TagName, target)
	return nil
}

func latestRelease(c *http.Client, repoAPI string) (*ghRelease, error) {
	resp, err := c.Get(strings.TrimRight(repoAPI, "/") + "/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release lookup failed: HTTP %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("no release found")
	}
	return &rel, nil
}

func findAsset(assets []ghAsset, name string) *ghAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// expectedSum downloads checksums.txt and returns the hex digest for name.
// goreleaser writes lines of the form "<sha256>  <filename>".
func expectedSum(c *http.Client, assets []ghAsset, name string) (string, error) {
	sums := findAsset(assets, "checksums.txt")
	if sums == nil {
		return "", nil // no checksums published
	}
	body, err := download(c, sums.URL)
	if err != nil {
		return "", err
	}
	return parseChecksums(string(body), name), nil
}

// parseChecksums returns the hex digest for name from goreleaser checksums.txt
// content ("<sha256>  <filename>" lines; "*" marks binary mode). "" if absent.
func parseChecksums(body, name string) string {
	for _, line := range strings.Split(body, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return f[0]
		}
	}
	return ""
}

func download(c *http.Client, url string) ([]byte, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20)) // 256 MiB ceiling
}

// extractBinary pulls the entry whose basename == binName from a .tar.gz archive.
func extractBinary(archive []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, 256<<20))
		if err != nil {
			return nil, fmt.Errorf("extract %s: %w", binName, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("binary %q not found in archive", binName)
}

// replaceSelf swaps the running executable, returning the path it wrote to.
// It first tries an in-place atomic rename (sibling temp → rename). If the
// install dir isn't writable (e.g. root-owned /usr/local/bin), harness doesn't
// need to live there, so it asks where to install: keep the system path (write
// it via sudo) or fall back to the user's bin path (no sudo).
func replaceSelf(data []byte, out io.Writer) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	if err := replaceInPlace(exe, data); err == nil {
		return exe, nil
	} else if !isPermission(err) || runtime.GOOS == "windows" {
		return "", err
	}

	// Permission denied on the current location. Ask where to install.
	userTarget := filepath.Join(userBinDir(), "vh-agent-harness")
	choice := "2"
	if !selfUpdateYes {
		fmt.Fprintf(out, "Cannot write %s (no permission). Choose install location:\n", exe)
		fmt.Fprintf(out, "  [1] System path  %s  (requires sudo)\n", exe)
		fmt.Fprintf(out, "  [2] User path    %s\n", userTarget)
		fmt.Fprint(out, "Choice [2]: ")
		var ans string
		_, _ = fmt.Scanln(&ans)
		if strings.TrimSpace(ans) == "1" {
			choice = "1"
		}
	}

	if choice == "1" {
		if err := replaceWithSudo(exe, data); err != nil {
			return "", err
		}
		return exe, nil
	}

	if err := writeToDir(userTarget, data); err != nil {
		return "", err
	}
	if exe != userTarget {
		fmt.Fprintf(out, "Note: the previous binary at %s is unchanged; ensure %s precedes it on PATH.\n", exe, filepath.Dir(userTarget))
	}
	return userTarget, nil
}

// userBinDir returns the user's binary directory ($XDG_BIN_HOME or ~/.local/bin).
func userBinDir() string {
	if d := os.Getenv("XDG_BIN_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/bin"
	}
	return filepath.Join(home, ".local", "bin")
}

// writeToDir atomically installs data as an executable at target, creating the
// parent directory if needed.
func writeToDir(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return replaceInPlace(target, data)
}

func isPermission(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

func replaceInPlace(exe string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".vh-agent-harness-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, exe)
}

func replaceWithSudo(exe string, data []byte) error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("cannot write %s and sudo is unavailable: run the update with sufficient privileges", exe)
	}
	// Stage in a writable temp dir, then `sudo install` over the target.
	tmp, err := os.CreateTemp("", "vh-agent-harness-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	cmd := exec.Command("sudo", "install", "-m", "0755", tmpName, exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr // let sudo prompt
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo install failed: %w", err)
	}
	return nil
}
