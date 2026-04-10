package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// BrunoRequest represents a Bruno request YAML structure
type BrunoRequest struct {
	Info struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
		Seq  int    `yaml:"seq"`
	} `yaml:"info"`
	HTTP struct {
		Method string `yaml:"method"`
		URL    string `yaml:"url"`
	} `yaml:"http"`
	Docs string `yaml:"docs"`
}

func main() {
	collectionDir := "docs/counter_api_bruno"
	outputFile := "docs/api.html"

	// Read all .yml files (except opencollection.yml)
	files, err := filepath.Glob(filepath.Join(collectionDir, "*.yml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding yml files: %v\n", err)
		os.Exit(1)
	}

	var requests []BrunoRequest
	for _, file := range files {
		if filepath.Base(file) == "opencollection.yml" {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file, err)
			continue
		}

		var req BrunoRequest
		if err := yaml.Unmarshal(data, &req); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			continue
		}

		requests = append(requests, req)
	}

	// Sort by sequence number
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].Info.Seq < requests[j].Info.Seq
	})

	// Generate HTML
	html := generateHTML(requests)

	// Write output
	if err := os.WriteFile(outputFile, []byte(html), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s with %d endpoints\n", outputFile, len(requests))
}

func generateHTML(requests []BrunoRequest) string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Counter API Documentation</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 40px 20px; text-align: center; }
        header h1 { font-size: 2.5em; margin-bottom: 10px; }
        header p { opacity: 0.9; font-size: 1.1em; }
        .toc { background: #f8f9fa; padding: 20px; border-radius: 8px; margin: 30px 0; }
        .toc h2 { margin-bottom: 15px; color: #667eea; }
        .toc ul { list-style: none; display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 10px; }
        .toc li { padding: 8px 12px; background: white; border-radius: 4px; border-left: 3px solid #667eea; }
        .toc a { text-decoration: none; color: #333; display: flex; align-items: center; gap: 10px; }
        .toc a:hover { color: #667eea; }
        .method { font-weight: bold; padding: 2px 8px; border-radius: 3px; font-size: 0.85em; color: white; }
        .method.GET { background: #61affe; }
        .method.POST { background: #49cc90; }
        .method.PUT { background: #fca130; }
        .method.DELETE { background: #f93e3e; }
        .endpoint { margin: 40px 0; padding: 30px; background: white; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        .endpoint-header { display: flex; align-items: center; gap: 15px; margin-bottom: 20px; padding-bottom: 15px; border-bottom: 2px solid #f0f0f0; }
        .endpoint-title { font-size: 1.8em; color: #333; }
        .endpoint-url { font-family: 'Monaco', 'Menlo', monospace; background: #f4f4f4; padding: 10px 15px; border-radius: 4px; margin: 15px 0; font-size: 0.95em; color: #e83e8c; }
        .endpoint-content h3 { color: #667eea; margin: 25px 0 15px 0; font-size: 1.3em; }
        .endpoint-content h4 { color: #49cc90; margin: 20px 0 10px 0; font-size: 1.1em; }
        .endpoint-content p { margin: 10px 0; }
        .endpoint-content ul { margin: 10px 0 10px 25px; }
        .endpoint-content li { margin: 8px 0; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; font-family: 'Monaco', 'Menlo', monospace; font-size: 0.9em; }
        pre { background: #2d2d2d; color: #f8f8f2; padding: 20px; border-radius: 6px; overflow-x: auto; margin: 15px 0; }
        pre code { background: transparent; padding: 0; color: inherit; }
        .auth-box { background: #fff3cd; border-left: 4px solid #ffc107; padding: 15px; margin: 15px 0; }
        .rate-limit-box { background: #d1ecf1; border-left: 4px solid #17a2b8; padding: 15px; margin: 15px 0; }
        .error-box { background: #f8d7da; border-left: 4px solid #dc3545; padding: 15px; margin: 15px 0; }
        @media (max-width: 768px) {
            .toc ul { grid-template-columns: 1fr; }
            .endpoint-header { flex-direction: column; align-items: flex-start; }
        }
    </style>
</head>
<body>
    <header>
        <h1>Counter API</h1>
        <p>A simple counter service with tenant isolation</p>
    </header>

    <div class="container">
        <div class="toc">
            <h2>Table of Contents</h2>
            <ul>
`)

	// Generate TOC
	for _, req := range requests {
		sb.WriteString(fmt.Sprintf("                <li><a href=\"#%s\"><span class=\"method %s\">%s</span> %s</a></li>\n",
			slugify(req.Info.Name),
			req.HTTP.Method,
			req.HTTP.Method,
			req.Info.Name))
	}

	sb.WriteString(`            </ul>
        </div>
`)

	// Generate endpoints
	for _, req := range requests {
		sb.WriteString(fmt.Sprintf(`        <div class="endpoint" id="%s">
            <div class="endpoint-header">
                <span class="method %s">%s</span>
                <h2 class="endpoint-title">%s</h2>
            </div>
            <div class="endpoint-url">%s</div>
            <div class="endpoint-content">
%s
            </div>
        </div>
`,
			slugify(req.Info.Name),
			req.HTTP.Method,
			req.HTTP.Method,
			req.Info.Name,
			req.HTTP.URL,
			renderMarkdown(req.Docs),
		))
	}

	sb.WriteString(`    </div>
</body>
</html>`)

	return sb.String()
}

func renderMarkdown(md string) string {
	// Simple markdown to HTML converter
	lines := strings.Split(md, "\n")
	var result strings.Builder
	inCodeBlock := false
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code block
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				result.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				result.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			result.WriteString(line + "\n")
			continue
		}

		// Headers
		if strings.HasPrefix(trimmed, "### ") {
			closeList(&result, &inList)
			content := strings.TrimPrefix(trimmed, "### ")
			result.WriteString(fmt.Sprintf("<h4>%s</h4>\n", content))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			closeList(&result, &inList)
			content := strings.TrimPrefix(trimmed, "## ")
			if content == "Errors" {
				result.WriteString(fmt.Sprintf("<h3>%s</h3>\n", content))
				result.WriteString("<div class=\"error-box\"><ul>\n")
				inList = true
			} else {
				result.WriteString(fmt.Sprintf("<h3>%s</h3>\n", content))
			}
			continue
		}

		// Bold lines (authentication, rate limit)
		if strings.HasPrefix(trimmed, "**") && strings.HasSuffix(trimmed, "**") {
			closeList(&result, &inList)
			content := strings.Trim(trimmed, "*")
			if strings.Contains(content, "Authentication") {
				result.WriteString(fmt.Sprintf("<div class=\"auth-box\"><strong>%s</strong></div>\n", content))
			} else if strings.Contains(content, "Rate Limit") {
				result.WriteString(fmt.Sprintf("<div class=\"rate-limit-box\"><strong>%s</strong></div>\n", content))
			} else {
				result.WriteString(fmt.Sprintf("<p><strong>%s</strong></p>\n", content))
			}
			continue
		}

		// List items
		if strings.HasPrefix(trimmed, "- ") {
			if !inList {
				result.WriteString("<ul>\n")
				inList = true
			}
			content := strings.TrimPrefix(trimmed, "- ")
			result.WriteString(fmt.Sprintf("<li>%s</li>\n", renderInlineMarkdown(content)))
			continue
		}

		// Regular lines
		content := renderInlineMarkdown(trimmed)
		if content != "" {
			closeList(&result, &inList)
			result.WriteString(fmt.Sprintf("<p>%s</p>\n", content))
		}
	}

	closeList(&result, &inList)
	return result.String()
}

func renderInlineMarkdown(text string) string {
	// Code
	text = strings.ReplaceAll(text, "`", "<code>")
	// Bold
	text = strings.ReplaceAll(text, "**", "<strong>")
	// Italic
	text = strings.ReplaceAll(text, "_", "<em>")
	return text
}

func closeList(sb *strings.Builder, inList *bool) {
	if *inList {
		sb.WriteString("</ul></div>\n")
		*inList = false
	}
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", "-"), "/", "-"))
}
