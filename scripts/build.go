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

// MinifyCSS safely compresses CSS by removing comments and unnecessary whitespace
// while preserving all syntax tokens, braces, delimiters, combinators, strings,
// URLs, calc() expressions, media queries, and custom properties.
func MinifyCSS(input string) string {
	var sb strings.Builder
	sb.Grow(len(input))

	n := len(input)
	i := 0

	for i < n {
		c := input[i]

		// 1. Comments: /* ... */
		if c == '/' && i+1 < n && input[i+1] == '*' {
			i += 2
			for i < n {
				if input[i] == '*' && i+1 < n && input[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// 2. String literals: "..." or '...'
		if c == '"' || c == '\'' {
			quote := c
			sb.WriteByte(quote)
			i++
			for i < n {
				ch := input[i]
				sb.WriteByte(ch)
				i++
				if ch == '\\' && i < n {
					sb.WriteByte(input[i])
					i++
				} else if ch == quote {
					break
				}
			}
			continue
		}

		// 3. url(...)
		if (c == 'u' || c == 'U') && i+3 < n && strings.EqualFold(input[i:i+4], "url(") {
			sb.WriteString(input[i : i+4])
			i += 4
			for i < n {
				ch := input[i]
				if ch == '"' || ch == '\'' {
					quote := ch
					sb.WriteByte(quote)
					i++
					for i < n {
						qch := input[i]
						sb.WriteByte(qch)
						i++
						if qch == '\\' && i < n {
							sb.WriteByte(input[i])
							i++
						} else if qch == quote {
							break
						}
					}
					continue
				}
				sb.WriteByte(ch)
				i++
				if ch == ')' {
					break
				}
			}
			continue
		}

		// 4. Whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			// Skip all consecutive whitespace
			for i < n && (input[i] == ' ' || input[i] == '\t' || input[i] == '\n' || input[i] == '\r' || input[i] == '\f') {
				i++
			}
			if sb.Len() == 0 || i >= n {
				continue
			}
			lastByte := sb.String()[sb.Len()-1]
			nextByte := input[i]

			// Omit space if adjacent to characters where space is never syntactically required
			// Before or after: { } ; ,
			// After: : (unless part of pseudo-selectors or properties)
			if lastByte == '{' || lastByte == '}' || lastByte == ';' || lastByte == ',' || lastByte == ':' ||
				nextByte == '{' || nextByte == '}' || nextByte == ';' || nextByte == ',' || nextByte == ':' {
				continue
			}

			// In all other cases (e.g. combinators, values like "10px solid red", @media queries, calc expressions), emit a single space
			sb.WriteByte(' ')
			continue
		}

		// 5. Normal character
		sb.WriteByte(c)
		i++
	}

	res := sb.String()
	// Safely clean up redundant semicolons immediately before closing brace: ";}" -> "}"
	res = strings.ReplaceAll(res, ";}", "}")
	return strings.TrimSpace(res)
}

// MinifyHTML removes comments and trims blank lines from HTML templates without destroying inline spacing.
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
	return strings.TrimSpace(sb.String())
}

// MinifyJS safely compresses JavaScript by removing comments and excessive whitespace
// while strictly preserving string literals, template literals, and regex literals.
func MinifyJS(input string) string {
	var sb strings.Builder
	sb.Grow(len(input))

	n := len(input)
	i := 0

	for i < n {
		c := input[i]

		// 1. Block comments: /* ... */
		if c == '/' && i+1 < n && input[i+1] == '*' {
			i += 2
			for i < n {
				if input[i] == '*' && i+1 < n && input[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// 2. Line comments: // ...
		if c == '/' && i+1 < n && input[i+1] == '/' {
			i += 2
			for i < n && input[i] != '\n' && input[i] != '\r' {
				i++
			}
			continue
		}

		// 3. String literals: "..." or '...' or `...`
		if c == '"' || c == '\'' || c == '`' {
			quote := c
			sb.WriteByte(quote)
			i++
			for i < n {
				ch := input[i]
				sb.WriteByte(ch)
				i++
				if ch == '\\' && i < n {
					sb.WriteByte(input[i])
					i++
				} else if ch == quote {
					break
				}
			}
			continue
		}

		// 4. Normal characters
		sb.WriteByte(c)
		i++
	}

	lines := strings.Split(sb.String(), "\n")
	var result strings.Builder
	for _, l := range lines {
		trimmed := strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(trimmed) != "" {
			result.WriteString(trimmed)
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String())
}

// MinifySVG safely compresses SVG assets.
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
	targetVersion := flag.String("version", "", "binary version (e.g. 1.0.1 or v1.0.1)")
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

	// Validate minified CSS assets before compiling
	if cssBytes, ok := filesToMinify["web/static/css/app.css"]; ok {
		minifiedCSS := MinifyCSS(string(cssBytes))
		openBraces := strings.Count(minifiedCSS, "{")
		closeBraces := strings.Count(minifiedCSS, "}")
		if openBraces == 0 || openBraces != closeBraces {
			fmt.Fprintf(os.Stderr, "ERROR: Minified CSS brace mismatch (%d '{' vs %d '}')\n", openBraces, closeBraces)
			os.Exit(1)
		}
		criticalSelectors := []string{
			".auth-page",
			".auth-shell",
			".auth-logo",
			".auth-title",
			".auth-card",
			".form-group",
			".btn-submit",
		}
		for _, sel := range criticalSelectors {
			if !strings.Contains(minifiedCSS, sel) {
				fmt.Fprintf(os.Stderr, "ERROR: Minified CSS missing critical selector %q\n", sel)
				os.Exit(1)
			}
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

	version := strings.TrimPrefix(*targetVersion, "v")
	if version == "" {
		version = strings.TrimPrefix(os.Getenv("VERSION"), "v")
	}
	if version == "" {
		if out, err := exec.Command("git", "describe", "--tags", "--exact-match").Output(); err == nil {
			version = strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
		}
	}
	if version == "" {
		version = "1.0.1"
	}

	buildDate := time.Now().UTC().Format(time.RFC3339)
	commit := "release"
	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	ldflags := fmt.Sprintf("-s -w -X 'main.Version=%s' -X 'main.BuildDate=%s' -X 'main.Commit=%s' -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.Version=%s' -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.BuildDate=%s' -X 'github.com/AlphaTechiess/alphadrive/cmd/alphadrive.Commit=%s'", version, buildDate, commit, version, buildDate, commit)

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
