package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BundleProject scans the codebase and returns a structured Markdown string for Gemini.
func BundleProject(rootPath string) (string, error) {
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

			// Format the relative path nicely for markdown
			relPath, err := filepath.Rel(rootPath, path)
			if err != nil {
				relPath = path
			}

			// Append structured Markdown formatting matching ExtractFilesFromMarkdown regex
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

type ExtractedFile struct {
	Name    string
	Content string
}

func ExtractFilesFromMarkdown(responseText string) []ExtractedFile {
	var files []ExtractedFile

	// Regex breakdown:
	// ### File:\s*`([^`]+)` -> Matches '### File: `filename.go`' capturing the name inside backticks
	// \s*```[a-zA-Z]*\r?\n  -> Matches the opening backticks and optional language identifier (like go, json, etc)
	// (.*?)                 -> Captures the inner content lazily (stopping at the next group)
	// \r?\n```              -> Matches the final closing backticks
	// (?:\r?\n### End of file:\s*`[^`]+`)? -> Matches the optional end of file marker
	pattern := `### ` + `File:\s*` + "`([^`]+)`" + `\s*` + "``" + `` + "`[a-zA-Z]*\r?\n([\\s\\S]*?)\r?\n" + "``" + `(?:\r?\n### End of file:\s*` + "`" + `[^`]+` + "`" + `)?`

	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(responseText, -1)

	for _, match := range matches {
		if len(match) == 3 {
			files = append(files, ExtractedFile{
				Name:    match[1], // Captured filename group
				Content: match[2], // Captured inner code content group
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
