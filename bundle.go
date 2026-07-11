package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundleProject scans the codebase and returns a structured Markdown string for Gemini.
func BundleProject(rootPath string, excludes []string) (string, error) {
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

		// Target Go source, modules, configuration, and documentation
		if ext == ".go" || ext == ".mod" || ext == ".sum" || ext == ".md" || ext == ".toml" || ext == ".yaml" || ext == ".json" {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files gracefully
			}

			// Append structured Markdown formatting matching ExtractFilesFromMarkdown
			builder.WriteString(fmt.Sprintf("### "+"File: `%s`\n", relPath))
			builder.WriteString("`" + "``" + strings.TrimPrefix(ext, ".") + "\n")
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

func shouldExclude(relPath string, info os.FileInfo, excludes []string) bool {
	// Normalize path to use forward slashes for uniform matching
	relPathNormalized := filepath.ToSlash(relPath)
	for _, pattern := range excludes {
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
	}
	return false
}

type ExtractedFile struct {
	Name    string
	Content string
}

// ExtractFilesFromMarkdown scans markdown text sequentially and extracts files matching the block structure.
// It checks that the optional '### End of file: filename' matches the opening '### File: filename'.
func ExtractFilesFromMarkdown(responseText string) []ExtractedFile {
	var files []ExtractedFile
	lines := strings.Split(responseText, "\n")
	n := len(lines)

	// Helper to extract file name inside backticks
	extractFilename := func(line, prefix string) (string, bool) {
		if !strings.HasPrefix(line, prefix) {
			return "", false
		}
		rem := strings.TrimPrefix(line, prefix)
		rem = strings.TrimSpace(rem)

		// Expecting backticks: `filename`
		if strings.HasPrefix(rem, "`") && strings.HasSuffix(rem, "`") {
			return strings.Trim(rem, "`"), true
		}
		return "", false
	}

	for i := 0; i < n; i++ {
		line := strings.TrimSpace(lines[i])
		//		println("==== line:", line)

		// Find opening marker
		filename, ok := extractFilename(line, "### File:")
		if !ok {
			continue
		}
		println("==== 1", filename)

		// Next line must be the opening fence
		if i+1 >= n {
			println("==== br", filename)
			break
		}
		i++
		openFence := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(openFence, "```") {
			continue
		}

		// Find content until closing fence
		var contentLines []string
		foundEndFence := false

		for i+1 < n {
			i++
			curLine := lines[i]
			trimmed := strings.TrimSpace(curLine)
			if trimmed == "```" {
				foundEndFence = true
				println("==== 4", filename)
				break
			}
			contentLines = append(contentLines, strings.TrimSuffix(curLine, "\r"))
		}

		if !foundEndFence {
			continue
		}

		// Check next line for the optional end of file marker
		if i+1 < n {
			nextIndex := i + 1
			endLine := strings.TrimSpace(lines[nextIndex])
			if strings.HasPrefix(endLine, "### End of file:") {
				endFilename, endOk := extractFilename(endLine, "### End of file:")
				if endOk {
					if endFilename != filename {
						fmt.Printf("Warning: End of file marker filename %q does not match opening filename %q. Skipping block.\n", endFilename, filename)
						continue
					}
					// Consume the end of file marker line since it matches
					i = nextIndex
				}
				println("==== 2", endLine)
			}
		}

		files = append(files, ExtractedFile{
			Name:    filename,
			Content: strings.Join(contentLines, "\n"),
		})
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
