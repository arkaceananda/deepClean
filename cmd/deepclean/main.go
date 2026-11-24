package main

import (
	"bufio"
	"deepClean/internal/core"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Version follows Semantic Versioning.
const Version = "0.3.1"

type Report struct {
	Version     string     `json:"version"`
	GeneratedAt string     `json:"generated_at"`
	Root        string     `json:"root"`
	MinSize     int64      `json:"min_size_bytes"`
	OlderThan   string     `json:"older_than"`
	TotalFiles  int        `json:"total_files"`
	TotalSize   string     `json:"total_size"`
	Files       []FileOut  `json:"files"`
	Orphans     OrphanInfo `json:"orphans"`
}

type FileOut struct {
	Path     string `json:"path"`
	Size     string `json:"size"`
	SizeB    int64  `json:"size_bytes"`
	LastUsed string `json:"last_used"`
	Modified string `json:"modified"`
	Category string `json:"category"`
	Risk     string `json:"risk"`
	System   bool   `json:"system_path"`
}

type OrphanInfo struct {
	DNF     string `json:"dnf_autoremove"`
	Flatpak string `json:"flatpak_unused"`
}

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	openPath := flag.String("open", "", "open a path in the default file manager and exit")
	root := flag.String("path", "/", "root path to scan")
	days := flag.Int("days", 90, "consider files unused if last used older than N days")
	minSizeMB := flag.Float64("min-size-mb", 1, "minimum file size in MB")
	topN := flag.Int("top", 200, "max number of files to list (0 = all)")
	jsonOut := flag.String("json", "", "write JSON report to file")
	skipOrphans := flag.Bool("skip-orphans", false, "skip dnf/flatpak orphan checks")
	riskFilter := flag.String("risk", "all", "filter by risk level: all|safe|caution|risky")
	deleteFlag := flag.Bool("delete", false, "interactively delete listed files (with confirmation)")
	forceFlag := flag.Bool("force", false, "allow deleting risky/system files (use with care)")
	flag.Parse()

	if *versionFlag {
		fmt.Println("deepclean", Version)
		return
	}
	if *openPath != "" {
		if err := openInFileManager(*openPath); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "open error:", err)
			os.Exit(1)
		}
		return
	}

	minSize := int64(*minSizeMB * 1e6)
	olderThan := time.Duration(*days) * 24 * time.Hour

	fmt.Printf("deepclean: scanning %s (unused > %d days, min size %.2f MB)\n", *root, *days, *minSizeMB)

	res, err := core.Scan(*root, minSize, olderThan, *topN)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "scan error:", err)
		os.Exit(1)
	}

	// optional risk filter
	if *riskFilter != "" && *riskFilter != "all" {
		filtered := res.Files[:0]
		var total int64
		for _, f := range res.Files {
			if f.Risk == *riskFilter {
				filtered = append(filtered, f)
				total += f.Size
			}
		}
		res.Files = filtered
		res.TotalSize = total
	}

	// category totals
	catTotals := map[string]int64{}
	for _, f := range res.Files {
		catTotals[f.Category] += f.Size
	}

	fmt.Printf("\nFound %d files, total %s\n", len(res.Files), core.FormatSize(res.TotalSize))
	fmt.Println("By category:")
	cats := make([]string, 0, len(catTotals))
	for c := range catTotals {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Printf("  %-8s %s\n", c, core.FormatSize(catTotals[c]))
	}

	fmt.Printf("\nTop %d by size:\n", len(res.Files))
	fmt.Printf("%-12s %-10s %-8s %-20s %s\n", "SIZE", "CATEGORY", "RISK", "LAST USED", "PATH")
	for _, f := range res.Files {
		lastUsed := f.Atime
		if f.Atime.IsZero() {
			lastUsed = f.ModTime
		}
		sysMark := ""
		if core.IsSystemPath(f.Path) {
			sysMark = " [SYS]"
		}
		fmt.Printf("%-12s %-10s %-8s %-20s %s%s\n",
			core.FormatSize(f.Size), f.Category, f.Risk, lastUsed.Format("2006-01-02"), f.Path, sysMark)
	}

	if *deleteFlag {
		fmt.Println("\n--- Interactive delete ---")
		reader := bufio.NewReader(os.Stdin)
		deleted, freed := 0, int64(0)
		for _, f := range res.Files {
			if f.Risk == "risky" && !*forceFlag {
				fmt.Printf("SKIP (risky): %s\n", f.Path)
				continue
			}
			if core.IsSystemPath(f.Path) && !*forceFlag {
				fmt.Printf("SKIP (system): %s\n", f.Path)
				continue
			}
			fmt.Printf("Delete %s (%s, %s)? [y/N]: ", f.Path, core.FormatSize(f.Size), f.Risk)
			ans, _ := reader.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			if ans != "y" && ans != "yes" {
				continue
			}
			if err := os.Remove(f.Path); err != nil {
				if os.IsPermission(err) {
					_, _ = fmt.Fprintf(os.Stderr, "delete error: %v (try running with sudo)\n", err)
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "delete error: %v\n", err)
				}
				continue
			}
			deleted++
			freed += f.Size
			fmt.Println("  deleted")
		}
		fmt.Printf("\nDeleted %d files, freed %s\n", deleted, core.FormatSize(freed))
	}

	if !*skipOrphans {
		fmt.Println("\n--- Package orphans (report only) ---")
		orphans := collectOrphans()
		if orphans.DNF != "" {
			fmt.Println("[dnf autoremove --assumeno]")
			fmt.Println(orphans.DNF)
		}
		if orphans.Flatpak != "" {
			fmt.Println("[flatpak list]")
			fmt.Println(orphans.Flatpak)
		}
	}

	if *jsonOut != "" {
		out := Report{
			Version:     Version,
			GeneratedAt: time.Now().Format(time.RFC3339),
			Root:        *root,
			MinSize:     minSize,
			OlderThan:   fmt.Sprintf("%d days", *days),
			TotalFiles:  len(res.Files),
			TotalSize:   core.FormatSize(res.TotalSize),
			Orphans:     collectOrphansNoExec(*skipOrphans),
		}
		for _, f := range res.Files {
			lastUsed := f.Atime
			if f.Atime.IsZero() {
				lastUsed = f.ModTime
			}
			out.Files = append(out.Files, FileOut{
				Path:     f.Path,
				Size:     core.FormatSize(f.Size),
				SizeB:    f.Size,
				LastUsed: lastUsed.Format(time.RFC3339),
				Modified: f.ModTime.Format(time.RFC3339),
				Category: f.Category,
				Risk:     f.Risk,
				System:   core.IsSystemPath(f.Path),
			})
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(*jsonOut, b, 0644); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "json write error:", err)
		} else {
			fmt.Printf("\nJSON report written to %s\n", *jsonOut)
		}
	}

	if len(res.Errors) > 0 {
		fmt.Printf("\n(%d permission/access errors skipped)\n", len(res.Errors))
	}
}

func collectOrphans() OrphanInfo {
	o := OrphanInfo{}
	if _, err := exec.LookPath("dnf"); err == nil {
		cmd := exec.Command("dnf", "autoremove", "--assumeno")
		out, _ := cmd.CombinedOutput()
		o.DNF = strings.TrimSpace(string(out))
	}
	if _, err := exec.LookPath("flatpak"); err == nil {
		// Flatpak has no dry-run; never auto-uninstall. List installed refs instead.
		cmd := exec.Command("flatpak", "list", "--columns=application,branch,origin")
		out, _ := cmd.CombinedOutput()
		o.Flatpak = "Installed refs (run `flatpak uninstall --unused` manually to remove unused):\n" + strings.TrimSpace(string(out))
	}
	return o
}

func collectOrphansNoExec(skip bool) OrphanInfo {
	if skip {
		return OrphanInfo{}
	}
	return collectOrphans()
}

// openInFileManager opens a path using the desktop default handler.
func openInFileManager(p string) error {
	for _, cmd := range []string{"xdg-open", "gio"} {
		if _, err := exec.LookPath(cmd); err == nil {
			var c *exec.Cmd
			if cmd == "gio" {
				c = exec.Command("gio", "open", p)
			} else {
				c = exec.Command(cmd, p)
			}
			return c.Start()
		}
	}
	return fmt.Errorf("no file manager opener found (xdg-open/gio)")
}
