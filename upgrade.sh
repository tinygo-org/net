#!/usr/bin/env bash
set -euo pipefail

##############################################################################
# TinyGo "net" package upgrade script
#
# Automates Step 1 & Step 2 of the README upgrade process:
#   Step 1: Backport differences from Go UPSTREAM to current CUR
#   Step 2: Generate comparison report of NEW vs UPSTREAM
#
# Usage:
#   ./upgrade.sh [--dry-run] [--cur VERSION] [--upstream VERSION]
#
# Examples:
#   ./upgrade.sh --dry-run                          # Preview what would change
#   ./upgrade.sh --cur 1.21.4 --upstream 1.26.2     # Perform the upgrade
#   ./upgrade.sh --dry-run --file dial.go            # Preview single file
##############################################################################

# ── Defaults ────────────────────────────────────────────────────────────────
CUR_VERSION="1.21.4"
UPSTREAM_VERSION="1.26.2"
DRY_RUN=false
SINGLE_FILE=""
TINYGO_NET_DIR="$(cd "$(dirname "$0")" && pwd)"
WORK_DIR="${TINYGO_NET_DIR}/.upgrade-work"
REPORT_DIR="${TINYGO_NET_DIR}/.upgrade-report"

# ── Parse arguments ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)    DRY_RUN=true; shift ;;
        --cur)        CUR_VERSION="$2"; shift 2 ;;
        --upstream)   UPSTREAM_VERSION="$2"; shift 2 ;;
        --file)       SINGLE_FILE="$2"; shift 2 ;;
        --help|-h)
            sed -n '3,/^$/p' "$0"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# ── Color helpers ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# ── File classification ─────────────────────────────────────────────────────
# Files entirely new to TinyGo (not from upstream) — never overwrite these.
TINYGO_ONLY_FILES=(
    "netdev.go"
    "tlssock.go"
    "README.md"
    "LICENSE"
)

# Files that are straight copies from upstream (no TINYGO modifications).
# These can be replaced directly from upstream.
UNMODIFIED_FILES=(
    "ip.go"
    "mac.go"
    "mac_test.go"
    "parse.go"
    "pipe.go"
    "http/clone.go"
    "http/cookie.go"
    "http/fs.go"
    "http/http.go"
    "http/jar.go"
    "http/method.go"
    "http/sniff.go"
    "http/status.go"
    "http/internal/ascii/print.go"
    "http/internal/ascii/print_test.go"
    "http/internal/chunked.go"
    "http/internal/chunked_test.go"
    "http/httptest/recorder.go"
    "http/httputil/dump.go"
    "http/httputil/httputil.go"
    "http/httputil/persist.go"
    "http/httputil/reverseproxy.go"
    "http/pprof/pprof.go"
)

# Files copied AND modified for TinyGo (contain // TINYGO markers).
# These need 3-way merge: upstream changes applied to TinyGo-modified version.
MODIFIED_FILES=(
    "dial.go"
    "interface.go"
    "iprawsock.go"
    "ipsock.go"
    "lookup.go"
    "lookup_unix.go"
    "lookup_windows.go"
    "net.go"
    "tcpsock.go"
    "udpsock.go"
    "unixsock.go"
    "http/client.go"
    "http/header.go"
    "http/pattern.go"
    "http/request.go"
    "http/response.go"
    "http/server.go"
    "http/transfer.go"
    "http/transport.go"
    "http/httptest/httptest.go"
    "http/httptest/server.go"
    "http/httptrace/trace.go"
)

# ── Map TinyGo paths to upstream Go stdlib paths ───────────────────────────
# TinyGo net/   -> Go src/net/
# TinyGo http/  -> Go src/net/http/
upstream_path() {
    local file="$1"
    if [[ "$file" == http/* ]]; then
        echo "src/net/${file}"
    else
        echo "src/net/${file}"
    fi
}

# ── Detect per-file CUR version from TINYGO header comment ─────────────────
detect_cur_version() {
    local file="$1"
    local header
    header=$(head -1 "$file" 2>/dev/null || true)
    if [[ "$header" =~ Go\ ([0-9]+\.[0-9]+\.[0-9]+) ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo "$CUR_VERSION"
    fi
}

# ── Download Go source tarball ──────────────────────────────────────────────
download_go_source() {
    local version="$1"
    local dest_dir="${WORK_DIR}/go${version}"

    if [[ -d "$dest_dir/src/net" ]]; then
        info "Go ${version} source already cached at ${dest_dir}" >&2
        return 0
    fi

    local tarball="${WORK_DIR}/go${version}.src.tar.gz"
    local url="https://go.dev/dl/go${version}.src.tar.gz"

    mkdir -p "$WORK_DIR"

    if [[ ! -f "$tarball" ]]; then
        info "Downloading Go ${version} source from ${url} ..."
        curl -fSL -o "$tarball" "$url"
    fi

    info "Extracting Go ${version} source..."
    mkdir -p "$dest_dir"
    tar xzf "$tarball" -C "$dest_dir" --strip-components=1
    success "Go ${version} source ready at ${dest_dir}"
}

# Try to use local gvm install first, fall back to download
resolve_go_source() {
    local version="$1"
    local gvm_path="${HOME}/.gvm/gos/go${version}"

    if [[ -d "${gvm_path}/src/net" ]]; then
        echo "$gvm_path"
        return 0
    fi

    # Check GOROOT if the active Go matches
    local goroot
    goroot="$(go env GOROOT 2>/dev/null || true)"
    if [[ -n "$goroot" && -d "${goroot}/src/net" ]]; then
        local active_ver
        active_ver="$(go version | grep -oP '\d+\.\d+\.\d+')"
        if [[ "$active_ver" == "$version" ]]; then
            echo "$goroot"
            return 0
        fi
    fi

    download_go_source "$version" >&2
    echo "${WORK_DIR}/go${version}"
}

# ── Check if a file exists upstream ─────────────────────────────────────────
file_exists_upstream() {
    local go_src_root="$1"
    local file="$2"
    local upath
    upath="$(upstream_path "$file")"
    [[ -f "${go_src_root}/${upath}" ]]
}

# ── Process unmodified (straight copy) files ────────────────────────────────
process_unmodified() {
    local file="$1"
    local upstream_root="$2"
    local upath
    upath="$(upstream_path "$file")"
    local upstream_file="${upstream_root}/${upath}"
    local tinygo_file="${TINYGO_NET_DIR}/${file}"

    if [[ ! -f "$upstream_file" ]]; then
        warn "MISSING in upstream ${UPSTREAM_VERSION}: ${upath}"
        echo "MISSING_UPSTREAM ${file}" >> "${REPORT_DIR}/summary.txt"
        return
    fi

    # Check if the file actually changed between versions
    local cur_root="$3"
    local cur_file="${cur_root}/${upath}"

    if [[ -f "$cur_file" ]] && diff -q "$cur_file" "$upstream_file" >/dev/null 2>&1; then
        echo "UNCHANGED ${file}" >> "${REPORT_DIR}/summary.txt"
        return
    fi

    if $DRY_RUN; then
        info "[DRY-RUN] Would replace ${file} with upstream ${UPSTREAM_VERSION} copy"
        if [[ -f "$cur_file" ]]; then
            diff -u "$cur_file" "$upstream_file" > "${REPORT_DIR}/diffs/${file}.diff" 2>/dev/null || true
            local lines
            lines=$(wc -l < "${REPORT_DIR}/diffs/${file}.diff" 2>/dev/null | tr -d ' ' || echo "0")
            info "  Upstream diff: ${lines} lines (see .upgrade-report/diffs/${file}.diff)"
        fi
        echo "WOULD_COPY ${file}" >> "${REPORT_DIR}/summary.txt"
    else
        # Update version header if it has one
        local header
        header=$(head -1 "$tinygo_file" 2>/dev/null || true)
        if [[ "$header" =~ ^//\ TINYGO ]]; then
            # Preserve TINYGO header, replace rest with upstream
            local old_ver
            old_ver=$(detect_cur_version "$tinygo_file")
            head -1 "$tinygo_file" > "${tinygo_file}.tmp"
            # Update version in header
            sed -i "s/Go ${old_ver}/Go ${UPSTREAM_VERSION}/" "${tinygo_file}.tmp"
            tail -n +2 "$upstream_file" >> "${tinygo_file}.tmp"
            mv "${tinygo_file}.tmp" "$tinygo_file"
        else
            cp "$upstream_file" "$tinygo_file"
        fi
        echo "COPIED ${file}" >> "${REPORT_DIR}/summary.txt"
        success "Copied ${file} from upstream ${UPSTREAM_VERSION}"
    fi
}

# ── Process modified (3-way merge) files ────────────────────────────────────
process_modified() {
    local file="$1"
    local upstream_root="$2"
    local cur_root="$3"

    local upath
    upath="$(upstream_path "$file")"
    local upstream_file="${upstream_root}/${upath}"
    local tinygo_file="${TINYGO_NET_DIR}/${file}"

    if [[ ! -f "$upstream_file" ]]; then
        warn "MISSING in upstream ${UPSTREAM_VERSION}: ${upath}"
        echo "MISSING_UPSTREAM ${file}" >> "${REPORT_DIR}/summary.txt"
        return
    fi

    # Detect the actual CUR version for this specific file
    local file_cur_version
    file_cur_version=$(detect_cur_version "$tinygo_file")

    # Get the CUR source root for this specific file version
    local file_cur_root
    if [[ "$file_cur_version" != "$CUR_VERSION" ]]; then
        info "${file}: based on Go ${file_cur_version} (not default ${CUR_VERSION})"
        file_cur_root=$(resolve_go_source "$file_cur_version")
        file_cur_root=$(echo "$file_cur_root" | tail -1)
    else
        file_cur_root="$cur_root"
    fi

    local cur_file="${file_cur_root}/${upath}"

    if [[ ! -f "$cur_file" ]]; then
        warn "MISSING in CUR ${file_cur_version}: ${upath} — can only diff against current TinyGo version"
        echo "MISSING_CUR ${file}" >> "${REPORT_DIR}/summary.txt"

        # Still generate a diff of upstream vs tinygo for manual review
        diff -u "$tinygo_file" "$upstream_file" > "${REPORT_DIR}/diffs/${file}.upstream-vs-tinygo.diff" 2>/dev/null || true
        return
    fi

    # Check if upstream even changed this file
    if diff -q "$cur_file" "$upstream_file" >/dev/null 2>&1; then
        echo "UNCHANGED ${file}" >> "${REPORT_DIR}/summary.txt"
        return
    fi

    # Generate upstream diff (what changed from CUR -> UPSTREAM in official Go)
    local upstream_diff="${REPORT_DIR}/diffs/${file}.upstream-changes.diff"
    diff -u "$cur_file" "$upstream_file" > "$upstream_diff" 2>/dev/null || true

    # Generate current tinygo diff (what TinyGo changed from CUR)
    local tinygo_diff="${REPORT_DIR}/diffs/${file}.tinygo-changes.diff"
    diff -u "$cur_file" "$tinygo_file" > "$tinygo_diff" 2>/dev/null || true

    local upstream_lines
    upstream_lines=$(wc -l < "$upstream_diff" 2>/dev/null | tr -d ' ' || echo "0")

    if $DRY_RUN; then
        info "[DRY-RUN] Would 3-way merge ${file} (Go ${file_cur_version} → ${UPSTREAM_VERSION})"
        info "  Upstream changes: ${upstream_lines} diff lines"
        info "  See: .upgrade-report/diffs/${file}.upstream-changes.diff"
        info "  See: .upgrade-report/diffs/${file}.tinygo-changes.diff"
        echo "WOULD_MERGE ${file} (${upstream_lines} upstream diff lines)" >> "${REPORT_DIR}/summary.txt"
    else
        # Attempt 3-way merge using diff3
        # Base = CUR upstream, Ours = TinyGo modified, Theirs = new upstream
        local merged="${REPORT_DIR}/merged/${file}"
        mkdir -p "$(dirname "$merged")"

        # diff3: merge changes from cur_file->upstream_file into tinygo_file
        # -m for merge mode, using tinygo_file as "ours"
        if diff3 -m "$tinygo_file" "$cur_file" "$upstream_file" > "$merged" 2>/dev/null; then
            # Clean merge
            cp "$merged" "$tinygo_file"

            # Update version header
            local old_ver
            old_ver=$(detect_cur_version "$tinygo_file")
            sed -i "1s/Go ${old_ver}/Go ${UPSTREAM_VERSION}/" "$tinygo_file"

            echo "MERGED_CLEAN ${file}" >> "${REPORT_DIR}/summary.txt"
            success "Merged ${file} cleanly (Go ${file_cur_version} → ${UPSTREAM_VERSION})"
        else
            # Merge conflicts — save the conflicted file for manual resolution
            cp "$merged" "${tinygo_file}.conflicted"

            # Count conflicts
            local conflicts
            conflicts=$(grep -c '^<<<<<<<' "$merged" 2>/dev/null || echo "0")

            echo "CONFLICTS ${file} (${conflicts} conflicts)" >> "${REPORT_DIR}/summary.txt"
            warn "CONFLICTS in ${file}: ${conflicts} conflict(s)"
            warn "  Conflicted merge saved to: ${file}.conflicted"
            warn "  Upstream changes: .upgrade-report/diffs/${file}.upstream-changes.diff"
            warn "  TinyGo changes:   .upgrade-report/diffs/${file}.tinygo-changes.diff"
        fi
    fi
}

# ── Main ────────────────────────────────────────────────────────────────────
main() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo " TinyGo net package upgrade"
    echo " CUR: Go ${CUR_VERSION}  →  UPSTREAM: Go ${UPSTREAM_VERSION}"
    if $DRY_RUN; then
        echo " Mode: DRY RUN (no files will be changed)"
    else
        echo " Mode: APPLY CHANGES"
    fi
    echo "═══════════════════════════════════════════════════════════════"
    echo ""

    # Set up report directory
    rm -rf "$REPORT_DIR"
    mkdir -p "${REPORT_DIR}/diffs" "${REPORT_DIR}/merged"

    # Resolve Go source trees
    info "Resolving Go ${CUR_VERSION} source..."
    local cur_root
    cur_root=$(resolve_go_source "$CUR_VERSION")
    cur_root=$(echo "$cur_root" | tail -1)
    info "  → ${cur_root}"

    info "Resolving Go ${UPSTREAM_VERSION} source..."
    local upstream_root
    upstream_root=$(resolve_go_source "$UPSTREAM_VERSION")
    upstream_root=$(echo "$upstream_root" | tail -1)
    info "  → ${upstream_root}"

    echo ""

    # Build file list
    local files_to_process=()
    if [[ -n "$SINGLE_FILE" ]]; then
        files_to_process=("$SINGLE_FILE")
    else
        files_to_process=("${UNMODIFIED_FILES[@]}" "${MODIFIED_FILES[@]}")
    fi

    # Process each file
    local total=${#files_to_process[@]}
    local idx=0

    for file in "${files_to_process[@]}"; do
        idx=$((idx + 1))
        echo -e "${BLUE}[${idx}/${total}]${NC} Processing ${file}..."

        # Skip TinyGo-only files
        local is_tinygo_only=false
        for tgo_file in "${TINYGO_ONLY_FILES[@]}"; do
            if [[ "$file" == "$tgo_file" ]]; then
                is_tinygo_only=true
                break
            fi
        done
        if $is_tinygo_only; then
            info "  Skipping (TinyGo-only file)"
            echo "SKIPPED_TINYGO_ONLY ${file}" >> "${REPORT_DIR}/summary.txt"
            continue
        fi

        # Create diff directory structure
        mkdir -p "$(dirname "${REPORT_DIR}/diffs/${file}")"

        # Is it a modified or unmodified file?
        local is_modified=false
        for mod_file in "${MODIFIED_FILES[@]}"; do
            if [[ "$file" == "$mod_file" ]]; then
                is_modified=true
                break
            fi
        done

        if $is_modified; then
            process_modified "$file" "$upstream_root" "$cur_root"
        else
            process_unmodified "$file" "$upstream_root" "$cur_root"
        fi
    done

    # ── Summary report ──────────────────────────────────────────────────────
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo " Summary"
    echo "═══════════════════════════════════════════════════════════════"

    if [[ -f "${REPORT_DIR}/summary.txt" ]]; then
        local unchanged merged_clean conflicts_count would_copy would_merge copied missing_upstream missing_cur
        unchanged=$(grep -c "^UNCHANGED " "${REPORT_DIR}/summary.txt" || true)
        merged_clean=$(grep -c "^MERGED_CLEAN " "${REPORT_DIR}/summary.txt" || true)
        conflicts_count=$(grep -c "^CONFLICTS " "${REPORT_DIR}/summary.txt" || true)
        would_copy=$(grep -c "^WOULD_COPY " "${REPORT_DIR}/summary.txt" || true)
        would_merge=$(grep -c "^WOULD_MERGE " "${REPORT_DIR}/summary.txt" || true)
        copied=$(grep -c "^COPIED " "${REPORT_DIR}/summary.txt" || true)
        missing_upstream=$(grep -c "^MISSING_UPSTREAM " "${REPORT_DIR}/summary.txt" || true)
        missing_cur=$(grep -c "^MISSING_CUR " "${REPORT_DIR}/summary.txt" || true)
        # Default to 0 if empty
        : "${unchanged:=0}" "${merged_clean:=0}" "${conflicts_count:=0}"
        : "${would_copy:=0}" "${would_merge:=0}" "${copied:=0}"
        : "${missing_upstream:=0}" "${missing_cur:=0}"

        echo ""
        echo "  Unchanged (no upstream changes):  ${unchanged}"
        if $DRY_RUN; then
            echo "  Would copy (unmodified):          ${would_copy}"
            echo "  Would merge (modified):           ${would_merge}"
        else
            echo "  Copied (unmodified):              ${copied}"
            echo "  Merged cleanly:                   ${merged_clean}"
            echo "  CONFLICTS (need manual fix):      ${conflicts_count}"
        fi
        [[ "$missing_upstream" -gt 0 ]] && echo "  Missing in upstream:              ${missing_upstream}"
        [[ "$missing_cur" -gt 0 ]]      && echo "  Missing in CUR baseline:          ${missing_cur}"

        if [[ "$conflicts_count" -gt 0 ]]; then
            echo ""
            warn "Files with merge conflicts:"
            grep "^CONFLICTS " "${REPORT_DIR}/summary.txt" | while read -r _ f rest; do
                echo "    ${f} ${rest}"
            done
        fi

        if $DRY_RUN && [[ "$would_merge" -gt 0 ]]; then
            echo ""
            info "Modified files that need merging:"
            grep "^WOULD_MERGE " "${REPORT_DIR}/summary.txt" | while read -r _ f rest; do
                echo "    ${f} ${rest}"
            done
        fi
    fi

    echo ""
    echo "  Full report: ${REPORT_DIR}/"
    echo "  Diffs:       ${REPORT_DIR}/diffs/"
    if ! $DRY_RUN; then
        echo "  Merged:      ${REPORT_DIR}/merged/"
    fi
    echo ""

    if $DRY_RUN; then
        info "Dry run complete. Review the report, then run without --dry-run to apply."
    else
        info "Upgrade applied. Review changes, resolve any conflicts, then test."
        echo ""
        echo "Next steps:"
        echo "  1. Review and resolve any .conflicted files"
        echo "  2. Check for files missing from upstream (may have been renamed/removed)"
        echo "  3. Verify TINYGO comments are preserved"
        echo "  4. Test with TinyGo example/net examples"
        echo "  5. Update README.md version references"
    fi
}

main "$@"
