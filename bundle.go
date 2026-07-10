package main

import (
	"fmt"
	"os"
	"path/filepath"
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

			// Append structured Markdown formatting
			builder.WriteString(fmt.Sprintf("## File: %s\n", relPath))
			builder.WriteString("```" + strings.TrimPrefix(ext, ".") + "\n")
			builder.Write(content)
			builder.WriteString("\n```\n\n")
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to bundle codebase: %w", err)
	}

	return builder.String(), nil
}
