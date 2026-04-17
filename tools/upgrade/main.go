package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Defaults ────────────────────────────────────────────────────────────────

const (
	defaultCurVersion      = "1.21.4"
	defaultUpstreamVersion = "1.26.2"
)

// ── Color helpers ───────────────────────────────────────────────────────────

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorBlue   = "\033[0;34m"
	colorReset  = "\033[0m"
)

func info(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, colorBlue+"[INFO]"+colorReset+" "+msg+"\n", args...)
}
func success(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, colorGreen+"[OK]"+colorReset+" "+msg+"\n", args...)
}
func warn(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, colorYellow+"[WARN]"+colorReset+" "+msg+"\n", args...)
}
func errorf(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, colorRed+"[ERROR]"+colorReset+" "+msg+"\n", args...)
}

// ── File classification ─────────────────────────────────────────────────────

// Files entirely new to TinyGo (not from upstream) — never overwrite these.
var tinygoOnlyFiles = map[string]bool{
	"netdev.go":  true,
	"tlssock.go": true,
	"README.md":  true,
	"LICENSE":    true,
}

// Files that are straight copies from upstream (no TINYGO modifications).
var unmodifiedFiles = []string{
	"ip.go",
	"mac.go",
	"mac_test.go",
	"parse.go",
	"pipe.go",
	"http/clone.go",
	"http/cookie.go",
	"http/fs.go",
	"http/http.go",
	"http/jar.go",
	"http/method.go",
	"http/sniff.go",
	"http/status.go",
	"http/internal/ascii/print.go",
	"http/internal/ascii/print_test.go",
	"http/internal/chunked.go",
	"http/internal/chunked_test.go",
	"http/httptest/recorder.go",
	"http/httputil/dump.go",
	"http/httputil/httputil.go",
	"http/httputil/persist.go",
	"http/httputil/reverseproxy.go",
	"http/pprof/pprof.go",
}

// Files copied AND modified for TinyGo (contain // TINYGO markers).
// These need 3-way merge: upstream changes applied to TinyGo-modified version.
var modifiedFiles = []string{
	"dial.go",
	"interface.go",
	"iprawsock.go",
	"ipsock.go",
	"lookup.go",
	"lookup_unix.go",
	"lookup_windows.go",
	"net.go",
	"tcpsock.go",
	"udpsock.go",
	"unixsock.go",
	"http/client.go",
	"http/header.go",
	"http/pattern.go",
	"http/request.go",
	"http/response.go",
	"http/server.go",
	"http/transfer.go",
	"http/transport.go",
	"http/httptest/httptest.go",
	"http/httptest/server.go",
	"http/httptrace/trace.go",
}

var modifiedFilesSet map[string]bool

func init() {
	modifiedFilesSet = make(map[string]bool, len(modifiedFiles))
	for _, f := range modifiedFiles {
		modifiedFilesSet[f] = true
	}
}

// ── Map TinyGo paths to upstream Go stdlib paths ───────────────────────────

func upstreamPath(file string) string {
	return "src/net/" + file
}

// ── Detect per-file CUR version from TINYGO header comment ─────────────────

var versionRe = regexp.MustCompile(`Go (\d+\.\d+\.\d+)`)

func detectCurVersion(filePath, fallback string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return fallback
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		if m := versionRe.FindStringSubmatch(scanner.Text()); m != nil {
			return m[1]
		}
	}
	return fallback
}

// ── Resolve Go source trees ────────────────────────────────────────────────

func resolveGoSource(version, workDir string) (string, error) {
	// Try gvm install
	gvmPath := filepath.Join(os.Getenv("HOME"), ".gvm", "gos", "go"+version)
	if isDir(filepath.Join(gvmPath, "src", "net")) {
		return gvmPath, nil
	}

	// Try current GOROOT if active Go version matches
	goroot, _ := execOutput("go", "env", "GOROOT")
	goroot = strings.TrimSpace(goroot)
	if goroot != "" && isDir(filepath.Join(goroot, "src", "net")) {
		activeVer, _ := execOutput("go", "version")
		if m := versionRe.FindStringSubmatch(activeVer); m != nil && m[1] == version {
			return goroot, nil
		}
	}

	// Download
	return downloadGoSource(version, workDir)
}

func downloadGoSource(version, workDir string) (string, error) {
	destDir := filepath.Join(workDir, "go"+version)
	if isDir(filepath.Join(destDir, "src", "net")) {
		info("Go %s source already cached at %s", version, destDir)
		return destDir, nil
	}

	url := fmt.Sprintf("https://go.dev/dl/go%s.src.tar.gz", version)
	tarball := filepath.Join(workDir, fmt.Sprintf("go%s.src.tar.gz", version))

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", fmt.Errorf("creating work dir: %w", err)
	}

	if _, err := os.Stat(tarball); os.IsNotExist(err) {
		info("Downloading Go %s source from %s ...", version, url)
		if err := downloadFile(tarball, url); err != nil {
			return "", fmt.Errorf("downloading %s: %w", url, err)
		}
	}

	info("Extracting Go %s source...", version)
	if err := extractTarGz(tarball, destDir); err != nil {
		return "", fmt.Errorf("extracting tarball: %w", err)
	}

	success("Go %s source ready at %s", version, destDir)
	return destDir, nil
}

func downloadFile(dest, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractTarGz(tarball, destDir string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Strip first component ("go/") to match --strip-components=1
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		target := filepath.Join(destDir, parts[1])

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// ── Process unmodified (straight copy) files ────────────────────────────────

type summaryEntry struct {
	status string
	file   string
	detail string
}

type upgrader struct {
	curVersion      string
	upstreamVersion string
	dryRun          bool
	tinygoNetDir    string
	workDir         string
	reportDir       string
	curRoot         string
	upstreamRoot    string
	summary         []summaryEntry
}

func (u *upgrader) addSummary(status, file, detail string) {
	u.summary = append(u.summary, summaryEntry{status, file, detail})
}

func (u *upgrader) processUnmodified(file string) {
	upath := upstreamPath(file)
	upstreamFile := filepath.Join(u.upstreamRoot, upath)
	tinygoFile := filepath.Join(u.tinygoNetDir, file)

	if !fileExists(upstreamFile) {
		warn("MISSING in upstream %s: %s", u.upstreamVersion, upath)
		u.addSummary("MISSING_UPSTREAM", file, "")
		return
	}

	curFile := filepath.Join(u.curRoot, upath)

	if fileExists(curFile) && filesEqual(curFile, upstreamFile) {
		u.addSummary("UNCHANGED", file, "")
		return
	}

	if u.dryRun {
		info("[DRY-RUN] Would replace %s with upstream %s copy", file, u.upstreamVersion)
		if fileExists(curFile) {
			diffPath := filepath.Join(u.reportDir, "diffs", file+".diff")
			writeDiff(curFile, upstreamFile, diffPath)
			lines := countLines(diffPath)
			info("  Upstream diff: %d lines (see .upgrade-report/diffs/%s.diff)", lines, file)
		}
		u.addSummary("WOULD_COPY", file, "")
	} else {
		header := firstLine(tinygoFile)
		if strings.HasPrefix(header, "// TINYGO") {
			oldVer := detectCurVersion(tinygoFile, u.curVersion)
			upstreamContent, err := os.ReadFile(upstreamFile)
			if err != nil {
				errorf("reading upstream file %s: %v", upstreamFile, err)
				return
			}
			// Update version in header and prepend to upstream content (minus first line)
			newHeader := strings.Replace(header, "Go "+oldVer, "Go "+u.upstreamVersion, 1)
			// Find the first newline in upstream content to skip its first line
			upstreamLines := string(upstreamContent)
			if idx := strings.Index(upstreamLines, "\n"); idx >= 0 {
				upstreamLines = upstreamLines[idx:]
			}
			if err := os.WriteFile(tinygoFile, []byte(newHeader+upstreamLines), 0o644); err != nil {
				errorf("writing %s: %v", tinygoFile, err)
				return
			}
		} else {
			if err := copyFile(upstreamFile, tinygoFile); err != nil {
				errorf("copying %s: %v", file, err)
				return
			}
		}
		u.addSummary("COPIED", file, "")
		success("Copied %s from upstream %s", file, u.upstreamVersion)
	}
}

// ── Process modified (3-way merge) files ────────────────────────────────────

func (u *upgrader) processModified(file string) {
	upath := upstreamPath(file)
	upstreamFile := filepath.Join(u.upstreamRoot, upath)
	tinygoFile := filepath.Join(u.tinygoNetDir, file)

	if !fileExists(upstreamFile) {
		warn("MISSING in upstream %s: %s", u.upstreamVersion, upath)
		u.addSummary("MISSING_UPSTREAM", file, "")
		return
	}

	// Detect the actual CUR version for this specific file
	fileCurVersion := detectCurVersion(tinygoFile, u.curVersion)
	fileCurRoot := u.curRoot

	if fileCurVersion != u.curVersion {
		info("%s: based on Go %s (not default %s)", file, fileCurVersion, u.curVersion)
		var err error
		fileCurRoot, err = resolveGoSource(fileCurVersion, u.workDir)
		if err != nil {
			errorf("resolving Go %s source: %v", fileCurVersion, err)
			return
		}
	}

	curFile := filepath.Join(fileCurRoot, upath)

	if !fileExists(curFile) {
		warn("MISSING in CUR %s: %s — can only diff against current TinyGo version", fileCurVersion, upath)
		u.addSummary("MISSING_CUR", file, "")
		diffPath := filepath.Join(u.reportDir, "diffs", file+".upstream-vs-tinygo.diff")
		writeDiff(tinygoFile, upstreamFile, diffPath)
		return
	}

	// Check if upstream even changed this file
	if filesEqual(curFile, upstreamFile) {
		u.addSummary("UNCHANGED", file, "")
		return
	}

	// Generate upstream diff (what changed from CUR -> UPSTREAM in official Go)
	upstreamDiffPath := filepath.Join(u.reportDir, "diffs", file+".upstream-changes.diff")
	writeDiff(curFile, upstreamFile, upstreamDiffPath)

	// Generate current tinygo diff (what TinyGo changed from CUR)
	tinygoDiffPath := filepath.Join(u.reportDir, "diffs", file+".tinygo-changes.diff")
	writeDiff(curFile, tinygoFile, tinygoDiffPath)

	upstreamLines := countLines(upstreamDiffPath)

	if u.dryRun {
		info("[DRY-RUN] Would 3-way merge %s (Go %s → %s)", file, fileCurVersion, u.upstreamVersion)
		info("  Upstream changes: %d diff lines", upstreamLines)
		info("  See: .upgrade-report/diffs/%s.upstream-changes.diff", file)
		info("  See: .upgrade-report/diffs/%s.tinygo-changes.diff", file)
		u.addSummary("WOULD_MERGE", file, fmt.Sprintf("(%d upstream diff lines)", upstreamLines))
	} else {
		// Attempt 3-way merge using diff3
		mergedPath := filepath.Join(u.reportDir, "merged", file)
		if err := os.MkdirAll(filepath.Dir(mergedPath), 0o755); err != nil {
			errorf("creating merge dir: %v", err)
			return
		}

		// diff3 -m tinygoFile curFile upstreamFile
		cmd := exec.Command("diff3", "-m", tinygoFile, curFile, upstreamFile)
		output, err := cmd.Output()

		// Write merged output regardless of exit code
		if writeErr := os.WriteFile(mergedPath, output, 0o644); writeErr != nil {
			errorf("writing merged file: %v", writeErr)
			return
		}

		if err == nil {
			// Clean merge
			if cpErr := copyFile(mergedPath, tinygoFile); cpErr != nil {
				errorf("copying merged file: %v", cpErr)
				return
			}

			// Update version header
			updateVersionHeader(tinygoFile, u.upstreamVersion)

			u.addSummary("MERGED_CLEAN", file, "")
			success("Merged %s cleanly (Go %s → %s)", file, fileCurVersion, u.upstreamVersion)
		} else {
			// Merge conflicts
			conflictedPath := tinygoFile + ".conflicted"
			if cpErr := copyFile(mergedPath, conflictedPath); cpErr != nil {
				errorf("copying conflicted file: %v", cpErr)
				return
			}

			conflicts := countOccurrences(mergedPath, "<<<<<<<")

			u.addSummary("CONFLICTS", file, fmt.Sprintf("(%d conflicts)", conflicts))
			warn("CONFLICTS in %s: %d conflict(s)", file, conflicts)
			warn("  Conflicted merge saved to: %s.conflicted", file)
			warn("  Upstream changes: .upgrade-report/diffs/%s.upstream-changes.diff", file)
			warn("  TinyGo changes:   .upgrade-report/diffs/%s.tinygo-changes.diff", file)
		}
	}
}

func updateVersionHeader(filePath, newVersion string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) < 1 {
		return
	}
	if m := versionRe.FindStringSubmatch(lines[0]); m != nil {
		lines[0] = strings.Replace(lines[0], "Go "+m[1], "Go "+newVersion, 1)
		os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
	}
}

// ── Summary ─────────────────────────────────────────────────────────────────

func (u *upgrader) printSummary() {
	counts := make(map[string]int)
	for _, e := range u.summary {
		counts[e.status]++
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println(" Summary")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("  Unchanged (no upstream changes):  %d\n", counts["UNCHANGED"])
	if u.dryRun {
		fmt.Printf("  Would copy (unmodified):          %d\n", counts["WOULD_COPY"])
		fmt.Printf("  Would merge (modified):           %d\n", counts["WOULD_MERGE"])
	} else {
		fmt.Printf("  Copied (unmodified):              %d\n", counts["COPIED"])
		fmt.Printf("  Merged cleanly:                   %d\n", counts["MERGED_CLEAN"])
		fmt.Printf("  CONFLICTS (need manual fix):      %d\n", counts["CONFLICTS"])
	}
	if counts["MISSING_UPSTREAM"] > 0 {
		fmt.Printf("  Missing in upstream:              %d\n", counts["MISSING_UPSTREAM"])
	}
	if counts["MISSING_CUR"] > 0 {
		fmt.Printf("  Missing in CUR baseline:          %d\n", counts["MISSING_CUR"])
	}

	if counts["CONFLICTS"] > 0 {
		fmt.Println()
		warn("Files with merge conflicts:")
		for _, e := range u.summary {
			if e.status == "CONFLICTS" {
				fmt.Printf("    %s %s\n", e.file, e.detail)
			}
		}
	}

	if u.dryRun && counts["WOULD_MERGE"] > 0 {
		fmt.Println()
		info("Modified files that need merging:")
		for _, e := range u.summary {
			if e.status == "WOULD_MERGE" {
				fmt.Printf("    %s %s\n", e.file, e.detail)
			}
		}
	}

	fmt.Println()
	fmt.Printf("  Full report: %s/\n", u.reportDir)
	fmt.Printf("  Diffs:       %s/diffs/\n", u.reportDir)
	if !u.dryRun {
		fmt.Printf("  Merged:      %s/merged/\n", u.reportDir)
	}
	fmt.Println()

	if u.dryRun {
		info("Dry run complete. Review the report, then run without --dry-run to apply.")
	} else {
		info("Upgrade applied. Review changes, resolve any conflicts, then test.")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review and resolve any .conflicted files")
		fmt.Println("  2. Check for files missing from upstream (may have been renamed/removed)")
		fmt.Println("  3. Verify TINYGO comments are preserved")
		fmt.Println("  4. Test with TinyGo example/net examples")
		fmt.Println("  5. Update README.md version references")
	}
}

// ── Write summary.txt ───────────────────────────────────────────────────────

func (u *upgrader) writeSummaryFile() {
	path := filepath.Join(u.reportDir, "summary.txt")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	for _, e := range u.summary {
		if e.detail != "" {
			fmt.Fprintf(f, "%s %s %s\n", e.status, e.file, e.detail)
		} else {
			fmt.Fprintf(f, "%s %s\n", e.status, e.file)
		}
	}
}

// ── Utility functions ───────────────────────────────────────────────────────

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func filesEqual(a, b string) bool {
	dataA, errA := os.ReadFile(a)
	dataB, errB := os.ReadFile(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(dataA) == string(dataB)
}

func firstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeDiff(fileA, fileB, outPath string) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return
	}
	cmd := exec.Command("diff", "-u", fileA, fileB)
	// diff returns exit code 1 when files differ, which is expected
	output, _ := cmd.CombinedOutput()
	os.WriteFile(outPath, output, 0o644)
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func countOccurrences(path, needle string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), needle)
}

func execOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	curVersion := flag.String("cur", defaultCurVersion, "Current Go version the TinyGo net package is based on")
	upstreamVersion := flag.String("upstream", defaultUpstreamVersion, "Target upstream Go version to upgrade to")
	dryRun := flag.Bool("dry-run", false, "Preview what would change without modifying files")
	singleFile := flag.String("file", "", "Process only a single file")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *help {
		fmt.Println(`TinyGo "net" package upgrade tool

Automates Step 1 & Step 2 of the README upgrade process:
  Step 1: Backport differences from Go UPSTREAM to current CUR
  Step 2: Generate comparison report of NEW vs UPSTREAM

Usage:
  upgrade [--dry-run] [--cur VERSION] [--upstream VERSION] [--file FILE]

Examples:
  upgrade --dry-run                          # Preview what would change
  upgrade --cur 1.21.4 --upstream 1.26.2     # Perform the upgrade
  upgrade --dry-run --file dial.go           # Preview single file`)
		os.Exit(0)
	}

	// Determine the TinyGo net directory (two levels up from tools/upgrade/)
	exe, err := os.Executable()
	if err != nil {
		// Fall back to working directory
		exe, _ = os.Getwd()
	} else {
		exe = filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	}
	// Also support running via "go run" — use working directory if exe path looks wrong
	tinygoNetDir := exe
	if !fileExists(filepath.Join(tinygoNetDir, "netdev.go")) {
		tinygoNetDir, _ = os.Getwd()
	}
	if !fileExists(filepath.Join(tinygoNetDir, "netdev.go")) {
		errorf("Cannot find TinyGo net directory. Run from the net package root or build the tool first.")
		os.Exit(1)
	}

	u := &upgrader{
		curVersion:      *curVersion,
		upstreamVersion: *upstreamVersion,
		dryRun:          *dryRun,
		tinygoNetDir:    tinygoNetDir,
		workDir:         filepath.Join(tinygoNetDir, ".upgrade-work"),
		reportDir:       filepath.Join(tinygoNetDir, ".upgrade-report"),
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println(" TinyGo net package upgrade")
	fmt.Printf(" CUR: Go %s  →  UPSTREAM: Go %s\n", u.curVersion, u.upstreamVersion)
	if u.dryRun {
		fmt.Println(" Mode: DRY RUN (no files will be changed)")
	} else {
		fmt.Println(" Mode: APPLY CHANGES")
	}
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Set up report directory
	os.RemoveAll(u.reportDir)
	os.MkdirAll(filepath.Join(u.reportDir, "diffs"), 0o755)
	os.MkdirAll(filepath.Join(u.reportDir, "merged"), 0o755)

	// Resolve Go source trees
	info("Resolving Go %s source...", u.curVersion)
	u.curRoot, err = resolveGoSource(u.curVersion, u.workDir)
	if err != nil {
		errorf("Failed to resolve Go %s: %v", u.curVersion, err)
		os.Exit(1)
	}
	info("  → %s", u.curRoot)

	info("Resolving Go %s source...", u.upstreamVersion)
	u.upstreamRoot, err = resolveGoSource(u.upstreamVersion, u.workDir)
	if err != nil {
		errorf("Failed to resolve Go %s: %v", u.upstreamVersion, err)
		os.Exit(1)
	}
	info("  → %s", u.upstreamRoot)
	fmt.Println()

	// Build file list
	var filesToProcess []string
	if *singleFile != "" {
		filesToProcess = []string{*singleFile}
	} else {
		filesToProcess = append(filesToProcess, unmodifiedFiles...)
		filesToProcess = append(filesToProcess, modifiedFiles...)
	}

	// Deduplicate and sort for deterministic order
	seen := make(map[string]bool)
	var deduped []string
	for _, f := range filesToProcess {
		if !seen[f] {
			seen[f] = true
			deduped = append(deduped, f)
		}
	}
	sort.Strings(deduped)
	filesToProcess = deduped

	total := len(filesToProcess)
	for idx, file := range filesToProcess {
		fmt.Printf(colorBlue+"[%d/%d]"+colorReset+" Processing %s...\n", idx+1, total, file)

		// Skip TinyGo-only files
		if tinygoOnlyFiles[file] {
			info("  Skipping (TinyGo-only file)")
			u.addSummary("SKIPPED_TINYGO_ONLY", file, "")
			continue
		}

		// Create diff directory structure
		os.MkdirAll(filepath.Dir(filepath.Join(u.reportDir, "diffs", file)), 0o755)

		if modifiedFilesSet[file] {
			u.processModified(file)
		} else {
			u.processUnmodified(file)
		}
	}

	u.writeSummaryFile()
	u.printSummary()
}
