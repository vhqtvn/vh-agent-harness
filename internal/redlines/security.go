package redlines

import (
	"os"
	"runtime"
)

// FileSecurityResult reports the secure-file posture of a registry file. It is
// advisory (WARN-level): the package NEVER fails a registry load solely on a
// permission finding — a too-open mode is surfaced so doctor can warn, but the
// registry is still trusted once it parses.
//
// This is a NEW, platform-aware contract authored for redlines. It does NOT
// inherit any posture from internal/originhash: that package's store is
// committed platform state written atomically but with no explicit 0600 check
// or doctor permission check (confirmed by reading originhash.go end-to-end).
// Origin hashes are not sensitive; redlines content is.
type FileSecurityResult struct {
	// Path is the file examined. Not sensitive.
	Path string
	// Checked is false on platforms where POSIX permission bits are not
	// meaningful (e.g. Windows). When false, all other fields are zero and the
	// result is a documented no-op.
	Checked bool
	// GroupOrWorldReadable is true when ANY group or other (world) read bit is
	// set on a POSIX platform. Because the registry may hold private terms,
	// such access is the condition a doctor check would WARN about.
	GroupOrWorldReadable bool
	// Mode is the raw permission bits (e.g. 0644) on POSIX platforms for
	// diagnostics. Zero when Checked is false.
	Mode os.FileMode
}

// modesAreMeaningful reports whether the current GOOS enforces POSIX-style
// permission bits. On Windows, file mode bits returned by os.Stat are not
// meaningful for access control (ACLs govern access), so the secure-file check
// is a no-op there.
func modesAreMeaningful() bool {
	switch runtime.GOOS {
	case "windows", "plan9", "js", "wasip1":
		return false
	default:
		return true
	}
}

// CheckFileSecurity examines a registry file's permissions. On POSIX platforms
// it reports whether the file is group- or world-readable (the WARN condition).
// On platforms without meaningful POSIX modes it returns Checked=false and
// performs no analysis. A missing file returns Checked=false (there is nothing
// to check; absence is the inert no-op case, not a permission problem).
//
// This helper is exposed for doctor wiring. It ships with tests; it is not
// invoked from Load (Load never fails solely on a permission finding).
func CheckFileSecurity(path string) FileSecurityResult {
	res := FileSecurityResult{Path: path}
	if !modesAreMeaningful() {
		return res
	}
	info, err := os.Stat(path)
	if err != nil {
		// Missing file: nothing to check. Absence is handled by Load as the
		// inert no-op; security posture is not a property of a non-existent
		// file.
		return res
	}
	mode := info.Mode().Perm()
	res.Checked = true
	res.Mode = mode
	// Any group or other access bit (0o077 = g+rwx | o+rwx). We warn on read
	// specifically, but flagging any group/other access is the conservative
	// posture — write/execute for group/other are worse than read.
	if mode&0o077 != 0 {
		res.GroupOrWorldReadable = true
	}
	return res
}
