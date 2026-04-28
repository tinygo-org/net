# upgrade

Automates the TinyGo `net` package upgrade process by backporting
changes from upstream Go stdlib and generating comparison reports.

It handles three categories of files:

- **TinyGo-only** (e.g. `netdev.go`, `tlssock.go`) — skipped entirely
- **Unmodified copies** — replaced directly from upstream
- **Modified files** (with `// TINYGO` markers) — 3-way merged via `diff3`

## Prerequisites

- `diff` and `diff3` (from GNU diffutils)
- Go source trees are resolved automatically:
  1. gvm installs at `~/.gvm/gos/`
  2. Active `GOROOT` if the version matches
  3. Downloaded from `https://go.dev/dl/` into `.upgrade-work/`

## Build

```sh
cd tools/upgrade
go build -o upgrade .
```

This produces the `upgrade` binary. It must be run from the `net`
package root directory (the repository root).

## Usage

```sh
# From the net package root:
./tools/upgrade/upgrade [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Preview changes without modifying files |
| `--cur` | `1.21.4` | Go version the TinyGo net package is currently based on |
| `--upstream` | `1.26.2` | Target upstream Go version to upgrade to |
| `--file` | | Process a single file instead of all files |

### Examples

```sh
# Preview what would change
./tools/upgrade/upgrade --dry-run

# Perform the upgrade
./tools/upgrade/upgrade --cur 1.21.4 --upstream 1.26.2

# Preview a single file
./tools/upgrade/upgrade --dry-run --file dial.go
```

## Output

Reports are written to `.upgrade-report/` in the net package root:

- `summary.txt` — one-line status per file
- `diffs/` — unified diffs showing upstream and TinyGo changes
- `merged/` — merge results (only in apply mode)

Files with merge conflicts are saved as `<file>.conflicted` alongside
the original.

## After upgrading

1. Review and resolve any `.conflicted` files
2. Check for files missing from upstream (may have been renamed/removed)
3. Verify `// TINYGO` comments are preserved
4. Test with TinyGo example/net examples
5. Update `README.md` version references
