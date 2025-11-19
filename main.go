package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

type FileDetail struct {
	Path string
	Size int64
}

type ExtensionStats struct {
	Sizes   map[string]int64
	Files   map[string][]FileDetail
	Folders map[string]map[string]int64
}

func newExtensionStats() *ExtensionStats {
	return &ExtensionStats{
		Sizes:   make(map[string]int64),
		Files:   make(map[string][]FileDetail),
		Folders: make(map[string]map[string]int64),
	}
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

func findExecutable(names ...string) (string, error) {
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("none of the executables were found")
}

func isStandardExtension(ext string) bool {
	if len(ext) > 4 {
		return false
	}
	hasLetter := false
	for _, r := range ext {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

// buildFdArgs builds the argument list for fdfind
func buildFdArgs(path, extensions string) []string {
	// always search all files, possibly narrowed by -e
	args := []string{"--type", "f", "-H", "-I", "--full-path", "--base-directory", path}

	if extensions == "" {
		return args
	}

	extensionList := strings.Split(extensions, ",")
	for _, ext := range extensionList {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			args = append(args, "-e", ext)
		}
	}
	return args
}

// scanFiles runs fdfind and fills ExtensionStats
func scanFiles(fdCmdName, path string, stats *ExtensionStats, cmdArgs []string) error {
	fdCmd := exec.Command(fdCmdName, cmdArgs...)

	stdout, err := fdCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stdout: %w", err)
	}
	stderr, err := fdCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stderr: %w", err)
	}

	if err := fdCmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	// logs fdfind stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("fd error output: %s\n", scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		relativePath := scanner.Text()
		filePath := filepath.Join(path, relativePath)
		info, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("Error statting file %s: %v\n", filePath, err)
			continue
		}

		fileExt := strings.ToLower(filepath.Ext(filePath))
		if fileExt == "" {
			fileExt = "no extension"
		} else {
			fileExt = fileExt[1:] // remove the dot
			if !isStandardExtension(fileExt) {
				fileExt = "no extension"
			}
		}

		fileSize := info.Size()
		stats.Sizes[fileExt] += fileSize
		stats.Files[fileExt] = append(stats.Files[fileExt], FileDetail{Path: filePath, Size: fileSize})

		dir := filepath.Dir(filePath)
		if _, exists := stats.Folders[fileExt]; !exists {
			stats.Folders[fileExt] = make(map[string]int64)
		}
		stats.Folders[fileExt][dir] += fileSize
	}

	if err := fdCmd.Wait(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	return nil
}

// collectSortedExtensions returns the list of known extensions, sorted according to flags
func collectSortedExtensions(sizes map[string]int64, sortName, reverseSize bool) []string {
	var exts []string
	for ext := range sizes {
		exts = append(exts, ext)
	}

	if sortName {
		sort.Strings(exts)
		return exts
	}

	if reverseSize {
		// smallest first
		sort.Slice(exts, func(i, j int) bool {
			return sizes[exts[i]] < sizes[exts[j]]
		})
	} else {
		// default = largest first
		sort.Slice(exts, func(i, j int) bool {
			return sizes[exts[i]] > sizes[exts[j]]
		})
	}

	return exts
}

// printDetails prints the per-extension "Storage Usage Per Extension" block
func printDetails(sortedExtensions []string, stats *ExtensionStats, detail, folderDetail bool, limit int, reverseSize bool) {
	if !detail && !folderDetail {
		return
	}

	fmt.Println("Storage Usage Per Extension:")
	for i, ext := range sortedExtensions {
		files := stats.Files[ext]
		size, exists := stats.Sizes[ext]
		if !exists || len(files) == 0 {
			fmt.Printf("%s: No files found.\n", strings.ToUpper(ext))
			continue
		}

		fmt.Printf("%s: %s\n", strings.ToUpper(ext), formatSize(size))

		if detail {
			// sort files by size in the same direction as summary
			if reverseSize {
				// -s = smallest first
				sort.Slice(files, func(i, j int) bool {
					return files[i].Size < files[j].Size
				})
			} else {
				// default = largest first
				sort.Slice(files, func(i, j int) bool {
					return files[i].Size > files[j].Size
				})
			}

			fileCount := len(files)
			displayLimit := limit
			if fileCount < limit {
				displayLimit = fileCount
			}
			for i := 0; i < displayLimit; i++ {
				prefix := "├──"
				if i == displayLimit-1 {
					prefix = "└──"
				}
				fmt.Printf("%s %s (%s)\n", prefix, files[i].Path, formatSize(files[i].Size))
			}
		}

		if folderDetail {
			fmt.Println("\nFolders:")
			folders := stats.Folders[ext]
			folderList := make([]FileDetail, 0, len(folders))
			for folder, fsize := range folders {
				folderList = append(folderList, FileDetail{Path: folder, Size: fsize})
			}

			if reverseSize {
				sort.Slice(folderList, func(i, j int) bool {
					return folderList[i].Size < folderList[j].Size
				})
			} else {
				sort.Slice(folderList, func(i, j int) bool {
					return folderList[i].Size > folderList[j].Size
				})
			}

			folderCount := len(folderList)
			folderDisplayLimit := limit
			if folderCount < limit {
				folderDisplayLimit = folderCount
			}
			for i := 0; i < folderDisplayLimit; i++ {
				prefix := "├──"
				if i == folderDisplayLimit-1 {
					prefix = "└──"
				}
				fmt.Printf("%s %s (%s)\n", prefix, folderList[i].Path, formatSize(folderList[i].Size))
			}
		}

		if i < len(sortedExtensions)-1 && (detail || folderDetail) {
			fmt.Println("_____________")
			fmt.Println()
		}
	}
}

// printSummary prints the final summary block (always printed if there are any files)
func printSummary(sortedExtensions []string, sizes map[string]int64, total bool) {
	fmt.Println("==================================")
	fmt.Println(" Summary: Storage per Extension ")
	fmt.Println("==================================")
	for _, ext := range sortedExtensions {
		fmt.Printf("%s: %s\n", strings.ToUpper(ext), formatSize(sizes[ext]))
	}
	fmt.Println("==================================")

	if total {
		var totalSize int64
		for _, size := range sizes {
			totalSize += size
		}
		fmt.Printf("Total : %s\n", formatSize(totalSize))
	}
}

func main() {
	var path string
	var extensions string
	var detail bool
	var folderDetail bool
	var limit int
	var sortName bool
	var reverseSize bool
	var total bool

	rootCmd := &cobra.Command{
		Use:   "extdust",
		Short: "Search for files with specific extensions and calculate total size per extension",
		Long:  `A simple CLI tool to search for files with given extensions starting from a specified path and display their total size per extension, with optional file or folder details.`,
		Run: func(cmd *cobra.Command, args []string) {
			if path == "" {
				p, err := os.Getwd()
				if err != nil {
					fmt.Printf("Error getting current directory: %v\n", err)
					os.Exit(1)
				}
				path = p
			}

			fdCmdName, err := findExecutable("fd", "fdfind")
			if err != nil {
				fmt.Println("Failed to find fdfind on your system. Please ensure it has been installed, and is in your PATH.")
				os.Exit(1)
			}

			stats := newExtensionStats()
			cmdArgs := buildFdArgs(path, extensions)

			if err := scanFiles(fdCmdName, path, stats, cmdArgs); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			// collect all extensions we saw
			if len(stats.Sizes) == 0 {
				fmt.Println("No files found.")
				return
			}

			sortedExtensions := collectSortedExtensions(stats.Sizes, sortName, reverseSize)

			// show the detailed per-extension block only when -f or -d is used
			// if the user just passes -e, we skip this and only show the summary
			if detail || folderDetail {
				printDetails(sortedExtensions, stats, detail, folderDetail, limit, reverseSize)
				fmt.Println()
			}

			// final summary
			printSummary(sortedExtensions, stats.Sizes, total)
		},
	}

	rootCmd.Flags().StringVarP(&path, "path", "p", "", "Path to search (default: current directory)")
	rootCmd.Flags().StringVarP(&extensions, "ext", "e", "", "Comma-separated file extensions to search for")

	rootCmd.Flags().BoolVarP(&detail, "files", "f", false, "Show file details per extension")
	rootCmd.Flags().BoolVarP(&folderDetail, "dirs", "d", false, "Show folder details per extension")

	rootCmd.Flags().IntVarP(&limit, "limit", "l", 100, "Limit the number of results displayed")

	rootCmd.Flags().BoolVarP(&reverseSize, "size", "s", false, "Sort by size, smallest first (default: largest first)")
	rootCmd.Flags().BoolVarP(&sortName, "name", "n", false, "Sort summary by extension name")

	rootCmd.Flags().BoolVarP(&total, "total", "t", false, "Show total size of all extensions combined")

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = false

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
