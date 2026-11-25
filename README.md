# deepClean

A Go command-line tool for discovering old, large, and unused files (and unused software packages) on Linux systems. Designed for Fedora first, with graceful degradation on other distributions.

> Status: early development (v0.8.0). Report-only by default — no files are deleted automatically.

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
go build -o deepclean ./cmd/deepclean
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

# Interactively delete candidates (never deletes risky/system unless --force)
./deepclean --path /tmp --days 0 --min-size-mb 1 --risk safe --delete

# Delete system cache (requires sudo)
sudo ./deepclean --path /var/cache/PackageKit --days 0 --min-size-mb 1 --risk safe --delete

# Move to Trash instead of permanent delete
./deepclean --path /home --days 60 --min-size-mb 50 --risk safe --trash --delete

# Quiet mode for cron/automation
./deepclean --path / --days 90 --min-size-mb 100 --risk safe --quiet --json ~/deepclean-reports/scan.json


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
| `--delete` | `false` | Interactively delete listed files (with confirmation) |
| `--trash` | `false` | Move files to Trash instead of permanent delete |
| `--force` | `false` | Allow deleting `risky`/system files (use with care) |
| `--quiet` | `false` | Quiet output (for cron/automation) |
| `--exit-code` | `false` | Exit with status 1 if candidates found (cron alerts) |
| `--set-telegram-token` | `` | Save Telegram bot token and exit |
| `--set-telegram-chat` | `` | Save Telegram chat id and exit |
| `--telegram-token` | `` | Override Telegram bot token |
| `--telegram-chat` | `` | Override Telegram chat id |

## Risk levels

| Level | Meaning | Examples |
|---|---|---|
| `risky` | System/protected paths; never auto-delete | `/usr`, `/etc`, `/boot`, `/lib`, `/bin`, `/sbin`, `/opt` |
| `caution` | Potentially useful but stale; review before deleting | build artifacts, `node_modules`, unknown files |
| `safe` | Regenerable cache/temp/log data | `/tmp`, `/var/tmp`, `*cache*`, `*.log` |

## Safety

- By default the tool is **report-only**: it never deletes files. Deletion requires `--delete` and per-file confirmation; `risky`/system paths are skipped unless `--force`.
- System directories are marked `[SYS]` and excluded from any future auto-cleanup.
- Special virtual filesystems (`/proc`, `/sys`, `/dev`, `/run`) are skipped.
- A full `/` scan emits permission errors without root; use `sudo` for complete results.

## Roadmap

- Risk rating per file (safe / caution / risky) beyond the `[SYS]` marker.
- Parallel directory traversal for faster full-system scans.
- Package manager support for Debian/Ubuntu (`apt`) and Arch (`pacman`).
- Windows support via build tags (currently Linux-only).
- Optional interactive deletion with confirmation.

## Telegram notifications

Setup (one-time, done by you):
1. Open Telegram, chat with **@BotFather**, run `/newbot`, follow prompts → you get a **bot token**.
2. Start a chat with your new bot (send any message).
3. Get your **chat id** (e.g. via `@userinfobot` or BotFather updates).
4. Save config:
   ```bash
   ./deepclean --set-telegram-token "BOT_TOKEN" --set-telegram-chat "CHAT_ID"
   ```

After that, any run with candidates sends a Telegram summary automatically (no flags needed).

Alternative: **ntfy.sh** is an even simpler HTTP push service (no account) if you prefer not using Telegram.

## TUI (config editor)

Build and run in a real terminal (not IDE run panel):

```bash
go build -o deepclean-tui ./cmd/deepclean-tui
./deepclean-tui
```

Navigasi: Tab/↑↓, Enter simpan, q keluar. Preferensi disimpan ke `~/.config/deepclean/config.json` dan dipakai CLI sebagai default.

## Automation

Example weekly cron (report only, quiet):

```bash
0 3 * * 0 /path/to/deepclean --path / --days 90 --min-size-mb 100 --risk safe --quiet --exit-code --json /home/$USER/deepclean-reports/scan-latest.json
```

## Notes

- Full `/` scan takes roughly 15–30 seconds.
- Deleting or scanning system paths (e.g. `/var/cache`) requires `sudo`.
- On non-Fedora distributions, filesystem scanning still works; package orphan checks are skipped when `dnf`/`flatpak` are unavailable.
- Flatpak has no dry-run mode, so only installed refs are listed; run `flatpak uninstall --unused` manually after review.
- Trash mode requires `gio` or `trash-put`.