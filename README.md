# deepClean

A Go command-line tool for discovering old, large, and unused files (and unused software packages) on Linux systems. Designed for Fedora first, with graceful degradation on other distributions.

> Status: early development (v0.2.0). Report-only by default — no files are deleted automatically.

## Features

- Recursive filesystem scan with configurable age and size thresholds.
- "Last used" derived from `max(atime, mtime)` to stay reliable on `relatime` mounts.
- Decimal size output in MB/GB (1 MB = 1,000,000 bytes), never MiB/KiB.
- Coarse risk rating per file: `safe`, `caution`, `risky` (system paths are `risky`).
- Open any path in the default file manager via `--open`.
- Categorization: `cache`, `log`, `temp`, `other`, with per-category totals.
- System paths (`/usr`, `/etc`, `/boot`, `/lib`, …) are flagged `[SYS]` and never auto-deleted.
- Package orphan inspection:
  - `dnf autoremove --assumeno` (safe dry-run) on Fedora.
  - Installed Flatpak refs listing (Flatpak has no dry-run mode).
- Human-readable table plus optional JSON report.

## Requirements

- Go 1.21+ (developed with Go 1.26).
- Linux (amd64). Filesystem atime access uses Linux `syscall.Stat_t`.

## Build

```bash
cd deepClean
go build -o deepclean .
```

## Usage

```bash
# Scan entire filesystem, files unused > 90 days, >= 100 MB
./deepclean --path / --days 90 --min-size-mb 100 --top 50

# Write a JSON report
./deepclean --path / --days 60 --min-size-mb 50 --json ./report.json

# Scan home directory, skip package checks
./deepclean --path /home --days 30 --min-size-mb 10 --skip-orphans

# Open a path in the default file manager
./deepclean --open /home/arkaaceananda/Downloads

# Only show safe (regenerable) files
./deepclean --path / --days 90 --min-size-mb 100 --risk safe
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--path` | `/` | Root path to scan |
| `--days` | `90` | Consider a file unused if last used older than N days |
| `--min-size-mb` | `1` | Minimum file size in MB |
| `--top` | `200` | Maximum number of files to list (`0` = all) |
| `--json` | `` | Write a JSON report to the given file |
| `--skip-orphans` | `false` | Skip dnf/flatpak orphan checks |
| `--version` | `false` | Print version and exit |
| `--open` | `` | Open a path in the default file manager and exit |
| `--risk` | `all` | Filter by risk level: `all`, `safe`, `caution`, `risky` |

## Risk levels

| Level | Meaning | Examples |
|---|---|---|
| `risky` | System/protected paths; never auto-delete | `/usr`, `/etc`, `/boot`, `/lib`, `/bin`, `/sbin`, `/opt` |
| `caution` | Potentially useful but stale; review before deleting | build artifacts, `node_modules`, unknown files |
| `safe` | Regenerable cache/temp/log data | `/tmp`, `/var/tmp`, `*cache*`, `*.log` |

## Safety

- The tool is **report-only**: it never deletes files.
- System directories are marked `[SYS]` and excluded from any future auto-cleanup.
- Special virtual filesystems (`/proc`, `/sys`, `/dev`, `/run`) are skipped.
- A full `/` scan emits permission errors without root; use `sudo` for complete results.

## Roadmap

- Risk rating per file (safe / caution / risky) beyond the `[SYS]` marker.
- Parallel directory traversal for faster full-system scans.
- Package manager support for Debian/Ubuntu (`apt`) and Arch (`pacman`).
- Windows support via build tags (currently Linux-only).
- Optional interactive deletion with confirmation.

## Notes

- Full `/` scan takes roughly 15–30 seconds.
- On non-Fedora distributions, filesystem scanning still works; package orphan checks are skipped when `dnf`/`flatpak` are unavailable.
- Flatpak has no dry-run mode, so only installed refs are listed; run `flatpak uninstall --unused` manually after review.