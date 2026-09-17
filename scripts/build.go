package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	cssCommentRe  = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	htmlCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)
	multiSpaceRe  = regexp.MustCompile(`\s+`)
	tagSpaceRe    = regexp.MustCompile(`>\s+<`)
)

func MinifyCSS(input string) string {
	// Strip comments
	out := cssCommentRe.ReplaceAllString(input, "")
	// Normalize whitespace
	out = multiSpaceRe.ReplaceAllString(out, " ")
	// Remove space around delimiters
	delims := []string{": ", " :", " {", "{ ", " }", "} ", "; ", " ;", ", ", " ,", " >", "> ", " +", "+ "}
	for i := 0; i < len(delims); i += 2 {
		out = strings.ReplaceAll(out, delims[i], string(delims[i][0]))
		out = strings.ReplaceAll(out, delims[i+1], string(delims[i+1][1]))
	}
	out = strings.ReplaceAll(out, ";}", "}")
	return strings.TrimSpace(out)
}

func MinifyHTML(input string) string {
	out := htmlCommentRe.ReplaceAllString(input, "")
	lines := strings.Split(out, "\n")
	var sb strings.Builder
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			sb.WriteString(trimmed)
			sb.WriteString("\n")
		}
	}
	res := sb.String()
	res = tagSpaceRe.ReplaceAllString(res, "><")
	return strings.TrimSpace(res)
}

func MinifyJS(input string) string {
	// Remove block comments
	out := cssCommentRe.ReplaceAllString(input, "")
	lines := strings.Split(out, "\n")
	var sb strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Strip line comments if not inside string
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func MinifySVG(input string) string {
	out := htmlCommentRe.ReplaceAllString(input, "")
	out = multiSpaceRe.ReplaceAllString(out, " ")
	out = strings.ReplaceAll(out, "> <", "><")
	return strings.TrimSpace(out)
}

func main() {
	outPath := flag.String("output", "dist/alphadrive", "output binary path")
	targetOS := flag.String("os", "", "target GOOS")
	targetArch := flag.String("arch", "", "target GOARCH")
	flag.Parse()

	// 1. Snapshot development files
	filesToMinify := map[string][]byte{}
	err := filepath.Walk("web", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext == ".html" || ext == ".css" || ext == ".js" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			filesToMinify[path] = data
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "read web files: %v\n", err)
		os.Exit(1)
	}

	// Snapshot images
	_ = filepath.Walk("images", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".svg" {
			data, err := os.ReadFile(path)
			if err == nil {
				filesToMinify[path] = data
			}
		}
		return nil
	})

	// Defer restoration of original readable files
	defer func() {
		for path, origData := range filesToMinify {
			_ = os.WriteFile(path, origData, 0644)
		}
	}()

	// 2. Write minified assets
	var origSize, minSize int
	for path, origData := range filesToMinify {
		origSize += len(origData)
		content := string(origData)
		var minified string
		ext := filepath.Ext(path)
		switch ext {
		case ".css":
			minified = MinifyCSS(content)
		case ".html":
			minified = MinifyHTML(content)
		case ".js":
			minified = MinifyJS(content)
		case ".svg":
			minified = MinifySVG(content)
		default:
			minified = content
		}
		minBytes := []byte(minified)
		minSize += len(minBytes)
		if err := os.WriteFile(path, minBytes, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write minified %s: %v\n", path, err)
			os.Exit(1)
		}
	}

	savedPercent := 100.0 * (1.0 - float64(minSize)/float64(origSize))
	fmt.Printf("Assets optimized: %d -> %d bytes (%.1f%% reduction)\n", origSize, minSize, savedPercent)

	// 3. Compile Go binary
	env := os.Environ()
	if *targetOS != "" {
		env = append(env, "GOOS="+*targetOS)
	}
	if *targetArch != "" {
		env = append(env, "GOARCH="+*targetArch)
	}
	env = append(env, "CGO_ENABLED=0")

	version := "1.0.0"
	buildDate := time.Now().UTC().Format(time.RFC3339)
	commit := "release"
	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	ldflags := fmt.Sprintf("-s -w -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.Version=%s' -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.BuildDate=%s' -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.Commit=%s'", version, buildDate, commit)

	_ = os.MkdirAll(filepath.Dir(*outPath), 0755)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags="+ldflags, "-o", *outPath, "cmd/alphadrive/main.go")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Built optimized production binary -> %s\n", *outPath)

	// Compute SHA256 checksum
	if binData, err := os.ReadFile(*outPath); err == nil {
		h := sha256.Sum256(binData)
		hashStr := hex.EncodeToString(h[:])
		baseName := filepath.Base(*outPath)
		checksumFile := filepath.Join(filepath.Dir(*outPath), "checksums.txt")
		existing, _ := os.ReadFile(checksumFile)
		lines := strings.Split(string(existing), "\n")
		var newLines []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasSuffix(l, " "+baseName) && !strings.HasSuffix(l, "  "+baseName) {
				newLines = append(newLines, l)
			}
		}
		newLines = append(newLines, fmt.Sprintf("%s  %s", hashStr, baseName))
		_ = os.WriteFile(checksumFile, []byte(strings.Join(newLines, "\n")+"\n"), 0644)
		fmt.Printf("Updated checksums.txt -> %s (%s)\n", baseName, hashStr[:12]+"...")
	}
}
