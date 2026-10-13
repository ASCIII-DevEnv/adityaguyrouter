package server

import (
	"antigravity-gateway/internal/model"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListProjects scans projects in projectsDir
func ListProjects(projectsDir string) []model.ProjectInfo {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var projects []model.ProjectInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}

		fullPath := filepath.Join(projectsDir, name)
		techStack := detectTechStack(fullPath)
		desc := extractDescription(fullPath)

		projects = append(projects, model.ProjectInfo{
			Name:        name,
			Path:        fullPath,
			TechStack:   techStack,
			Description: desc,
		})
	}
	return projects
}

// InspectProject extracts key architecture files (README.md, AGENTS.md, package.json, go.mod, etc.)
func InspectProject(projPath, projName string) *model.ProjectContextSummary {
	techStack := detectTechStack(projPath)

	var keyFiles []string
	var summaryParts []string

	// Check instruction and context files in priority order
	priorityFiles := []string{
		"AGENTS.md",
		".hermes.md",
		"README.md",
		"DESIGN.md",
		"package.json",
		"go.mod",
		"requirements.txt",
		"pyproject.toml",
		"docker-compose.yml",
	}

	for _, relFile := range priorityFiles {
		filePath := filepath.Join(projPath, relFile)
		if fi, err := os.Stat(filePath); err == nil && !fi.IsDir() {
			keyFiles = append(keyFiles, relFile)
			// Read up to 10 KB per file
			if data, err := os.ReadFile(filePath); err == nil {
				content := string(data)
				const maxLen = 8000
				if len(content) > maxLen {
					content = content[:maxLen] + "\n...[truncated]..."
				}
				summaryParts = append(summaryParts, fmt.Sprintf("--- [%s] ---\n%s", relFile, content))
			}
		}
	}

	// Also scan top-level directory structure
	if entries, err := os.ReadDir(projPath); err == nil {
		var tree []string
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" || n == "vendor" || n == "dist" || n == "__pycache__" {
				continue
			}
			if e.IsDir() {
				tree = append(tree, n+"/")
			} else {
				tree = append(tree, n)
			}
		}
		if len(tree) > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("--- [Directory Listing] ---\n%s", strings.Join(tree, ", ")))
		}
	}

	return &model.ProjectContextSummary{
		ProjectName: projName,
		TechStack:   techStack,
		KeyFiles:    keyFiles,
		SummaryText: strings.Join(summaryParts, "\n\n"),
	}
}

func detectTechStack(dir string) string {
	var stacks []string

	if fileExists(filepath.Join(dir, "package.json")) {
		stacks = append(stacks, "Node.js/TypeScript")
	}
	if fileExists(filepath.Join(dir, "go.mod")) {
		stacks = append(stacks, "Go")
	}
	if fileExists(filepath.Join(dir, "requirements.txt")) || fileExists(filepath.Join(dir, "pyproject.toml")) {
		stacks = append(stacks, "Python")
	}
	if fileExists(filepath.Join(dir, "Dockerfile")) || fileExists(filepath.Join(dir, "docker-compose.yml")) {
		stacks = append(stacks, "Docker")
	}
	if fileExists(filepath.Join(dir, "pubspec.yaml")) {
		stacks = append(stacks, "Flutter/Dart")
	}

	if len(stacks) == 0 {
		return "Generic"
	}
	return strings.Join(stacks, ", ")
}

func extractDescription(dir string) string {
	readmePath := filepath.Join(dir, "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "!") && !strings.HasPrefix(trimmed, "[") {
				if len(trimmed) > 120 {
					return trimmed[:117] + "..."
				}
				return trimmed
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
