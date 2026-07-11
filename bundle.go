package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundleProject scans the codebase and returns a structured Markdown string for Gemini.
func BundleProject(rootPath string, excludes []string, includes []string) (string, error) {
	var builder strings.Builder

	// Write system context header
	builder.WriteString("# PROJECT CODEBASE CONTEXT\n\n")

	// Configuration maps for easy filtering
	skipDirs := map[string]bool{".git": true, "vendor": true, "node_modules": true, ".idea": true, ".vscode": true}
	skipExts := map[string]bool{".exe": true, ".png": true, ".jpg": true, ".ico": true, ".zip": true}

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Format the relative path nicely for markdown
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			relPath = path
		}

		// Check custom excludes first
		if shouldExclude(relPath, info, excludes) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Filter out dependency and system directories
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files starting with an underscore (_)
		if strings.HasPrefix(info.Name(), "_") {
			return nil
		}

		// Skip heavy binary files
		ext := strings.ToLower(filepath.Ext(path))
		if skipExts[ext] {
			return nil
		}

		// If include patterns are specified, ignore any files that do not match
		if len(includes) > 0 {
			if !matchesPatterns(relPath, info, includes) {
				return nil
			}
		}

		// Target files based on patterns or default whitelist
		isTarget := false
		if len(includes) > 0 {
			// Included explicitly by the user, mark as target
			isTarget = true
		} else {
			// Target Go source, modules, configuration, and documentation by default
			if ext == ".go" || ext == ".mod" || ext == ".sum" || ext == ".md" || ext == ".toml" || ext == ".yaml" || ext == ".json" {
				isTarget = true
			}
		}

		if isTarget {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files gracefully
			}

			// Format syntax highlighter block extension
			syntaxExt := strings.TrimPrefix(ext, ".")
			if syntaxExt == "" {
				syntaxExt = "text"
			}

			// Append structured Markdown formatting matching ExtractFilesFromMarkdown
			builder.WriteString(fmt.Sprintf("### "+"File: `%s`\n", relPath))
			builder.WriteString("`" + "``" + syntaxExt + "\n")
			builder.Write(content)
			builder.WriteString("\n" + "`" + "``\n")
			builder.WriteString(fmt.Sprintf("### End of file: `%s`\n\n", relPath))
			fmt.Printf("Bundle: %s\n", relPath)
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to bundle codebase: %w", err)
	}

	return builder.String(), nil
}

func matchesPatterns(relPath string, info os.FileInfo, patterns []string) bool {
	// Normalize path to use forward slashes for uniform matching
	relPathNormalized := filepath.ToSlash(relPath)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		patternNormalized := filepath.ToSlash(pattern)

		// Check direct match on normalized relative path
		if matched, _ := filepath.Match(patternNormalized, relPathNormalized); matched {
			return true
		}
		// Check match on filename only
		if matched, _ := filepath.Match(patternNormalized, info.Name()); matched {
			return true
		}
		// Check if any of the path segments match the pattern
		segments := strings.Split(relPathNormalized, "/")
		for _, seg := range segments {
			if matched, _ := filepath.Match(patternNormalized, seg); matched {
				return true
			}
		}
		// Check if prefix of relative path matches (to support directory inclusion)
		if strings.HasPrefix(relPathNormalized, patternNormalized+"/") {
			return true
		}
	}
	return false
}

func shouldExclude(relPath string, info os.FileInfo, excludes []string) bool {
	return matchesPatterns(relPath, info, excludes)
}

type ExtractedFile struct {
	Name    string
	Content string
}

// ExtractFilesFromMarkdown scans markdown text sequentially and extracts files matching the block structure.
// It reads the list of changed files first, then extracts the contents for those files.
func ExtractFilesFromMarkdown(responseText string) []ExtractedFile {
	var files []ExtractedFile

	// 1. Extract list of expected files from "### List of changed files:" block
	var expectedFiles []string
	startMarker := "### List of changed files:"
	endMarker := "### End the list of changed files"

	startIdx := strings.Index(responseText, startMarker)
	if startIdx != -1 {
		rest := responseText[startIdx+len(startMarker):]
		endIdx := strings.Index(rest, endMarker)
		if endIdx != -1 {
			listContent := rest[:endIdx]
			for _, line := range strings.Split(listContent, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Clean list items: remove markdown bullet points and backticks
				line = strings.TrimPrefix(line, "-")
				line = strings.TrimPrefix(line, "*")
				line = strings.TrimSpace(line)
				line = strings.Trim(line, "`")
				line = strings.TrimSpace(line)
				if line != "" {
					expectedFiles = append(expectedFiles, line)
				}
			}
		}
	}

	// 2. If no list of files was parsed, fallback: scan the text sequentially for any "### File:" markers
	if len(expectedFiles) == 0 {
		lines := strings.Split(responseText, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "### File:") {
				filename := strings.TrimPrefix(line, "### File:")
				filename = strings.TrimSpace(filename)
				filename = strings.Trim(filename, "`")
				filename = strings.TrimSpace(filename)
				if filename != "" {
					expectedFiles = append(expectedFiles, filename)
				}
			}
		}
	}

	// Remove potential duplicates from expectedFiles
	seen := make(map[string]bool)
	var uniqueFiles []string
	for _, f := range expectedFiles {
		if !seen[f] {
			seen[f] = true
			uniqueFiles = append(uniqueFiles, f)
		}
	}

	// 3. Cut files based on the expected filenames list
	for _, filename := range uniqueFiles {
		// Match "### File: `filename`" or "### File: filename"
		markerWithBackticks := fmt.Sprintf("### File: `%s`", filename)
		markerWithoutBackticks := fmt.Sprintf("### File: %s", filename)

		idx := strings.Index(responseText, markerWithBackticks)
		if idx == -1 {
			idx = strings.Index(responseText, markerWithoutBackticks)
		}
		if idx == -1 {
			continue
		}

		// Search from the start of the file block
		contentStart := responseText[idx:]
		lines := strings.Split(contentStart, "\n")

		// Find opening code fence ```
		fenceIdx := -1
		for i := 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, "```") {
				fenceIdx = i
				break
			}
		}
		if fenceIdx == -1 {
			continue
		}

		// Accumulate lines until closing code fence ```
		var contentLines []string
		foundEnd := false
		for i := fenceIdx + 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "```" {
				foundEnd = true
				break
			}
			contentLines = append(contentLines, strings.TrimSuffix(lines[i], "\r"))
		}

		if foundEnd {
			files = append(files, ExtractedFile{
				Name:    filename,
				Content: strings.Join(contentLines, "\n"),
			})
		}
	}

	return files
}

func ExtractCommitMessage(responseText string) string {
	marker := "### Proposed " + "commit message:"
	idx := strings.LastIndex(responseText, marker)
	if idx == -1 {
		return ""
	}
	content := responseText[idx+len(marker):]
	lines := strings.Split(content, "\n")
	var msgLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "###") {
			break
		}
		msgLines = append(msgLines, line)
	}
	msg := strings.TrimSpace(strings.Join(msgLines, "\n"))
	if strings.HasPrefix(msg, "``"+"`") {
		if firstNL := strings.Index(msg, "\n"); firstNL != -1 {
			msg = msg[firstNL+1:]
		}
		if strings.HasSuffix(msg, "``"+"`") {
			msg = strings.TrimSuffix(msg, "``"+"`")
		}
		msg = strings.TrimSpace(msg)
	}
	return msg
}

func WriteFilesToDisk(baseDir string, files []ExtractedFile) error {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path of base directory %s: %w", baseDir, err)
	}

	for _, file := range files {
		// Clean and secure the target file path relative to your base directory
		targetPath := filepath.Join(baseDir, filepath.Clean(file.Name))

		absTarget, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path of target file %s: %w", file.Name, err)
		}

		// Security Check: Verify absolute targetPath is within absolute baseDir to prevent directory traversal
		rel, err := filepath.Rel(absBase, absTarget)
		if err != nil {
			return fmt.Errorf("failed to determine relative path for %s: %w", file.Name, err)
		}
		if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("security violation: target path %q resolves outside the project directory %q", file.Name, baseDir)
		}

		// 1. Extract the file's parent directory path
		dir := filepath.Dir(absTarget)

		// 2. Recursively create any missing nested folders (e.g., pkg/server/)
		// 0755 provides read/write/execute for owner, read/execute for others
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory tree %s: %w", dir, err)
		}

		// 3. Write the code payload to the file
		// 0644 provides read/write for owner, read-only for others
		err = os.WriteFile(absTarget, []byte(file.Content+"\n"), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", absTarget, err)
		}

		fmt.Printf("Successfully written: %s\n", absTarget)
	}
	return nil
}
