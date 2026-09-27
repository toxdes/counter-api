package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
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

	fmt.Printf("Generated %s with %d homepage endpoints\n", outputFile, len(homepageRequests(requests)))
}

func generateHTML(requests []BrunoRequest) string {
	requests = homepageRequests(requests)
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="theme-color" content="#181828">
    <title>Counter API · Documentation</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root { color-scheme:dark; --ink:#ede7de; --muted:#a8b5a8; --line:#3d4748; --ground:#181828; --panel:#182121; --accent:#e99957; --accent-hover:#f1ae61; --code:#111719; font-family:"Inter",system-ui,sans-serif; font-size:16px; }
        * { box-sizing:border-box; } html { background:var(--ground); scroll-behavior:smooth; }
        body { margin:0; background:var(--ground); color:var(--ink); font-size:1rem; line-height:1.6; }
        a { color:var(--accent-hover); text-underline-offset:4px; } a:hover { color:var(--ink); }
        a:focus-visible,summary:focus-visible { outline:2px solid #e9a36c; outline-offset:4px; }
        .topbar { }
        .topbar-inner,main { width:min(1180px,calc(100% - 64px)); margin:auto; }
        .topbar-inner { min-height:68px; padding:8px 0 12px; display:flex; align-items:center; justify-content:space-between; border-bottom:1px solid var(--line); }
        .brand { color:var(--ink); font-weight:700; font-size:1.2rem; text-decoration:none; letter-spacing:-.03em; }
        .topbar nav { display:flex; gap:24px; } .topbar nav a { color:var(--muted); font-size:.92rem; }
        main { padding:62px 0 90px; } .hero { max-width:760px; padding-bottom:40px; }
        h1 { max-width:19ch; margin:0; font-size:clamp(2.5rem,3.55vw,3.3rem); line-height:1.2; letter-spacing:-.035em; }
        .lead { max-width:68ch; margin:14px 0 0; color:var(--muted); font-size:1rem; line-height:1.6; }
        .hero-actions { display:flex; align-items:center; flex-wrap:wrap; gap:22px; margin-top:24px; }
        .primary-link { display:inline-flex; min-height:46px; align-items:center; padding:0 17px; border-radius:4px; color:#191a20; background:var(--accent); font-weight:700; text-decoration:none; }
        .primary-link:hover { color:#191a20; background:var(--accent-hover); }
        .security { display:flex; gap:22px; flex-wrap:wrap; border-block:1px solid var(--line); padding:18px 0; color:var(--muted); font-size:.92rem; }
        .security strong { color:var(--ink); }
        .section-heading { display:flex; justify-content:space-between; align-items:baseline; gap:20px; margin:54px 0 22px; }
        h2 { margin:0; font-size:1.5rem; letter-spacing:-.025em; }
        .reference-layout { display:grid; grid-template-columns:210px minmax(0,1fr); align-items:start; gap:42px; }
        .toc { position:sticky; top:24px; display:grid; gap:3px; max-height:calc(100vh - 48px); overflow-y:auto; padding:0 0 8px; scrollbar-color:#566263 var(--ground); }
        .toc a { display:flex; align-items:center; min-height:40px; padding:7px 12px; border-left:2px solid transparent; color:var(--muted); font-size:.9rem; line-height:1.35; text-decoration:none; }
        .toc a:hover { color:var(--ink); }
        .toc a[aria-current="location"] { border-color:var(--accent); color:var(--ink); }
        .endpoint-list { min-width:0; }
        .endpoint { border-top:1px solid var(--line); padding:20px 0 24px; scroll-margin-top:24px; }
        .endpoint-header { display:flex; align-items:center; gap:13px; }
        .method { min-width:55px; color:var(--accent); font:700 .78rem/1 "JetBrains Mono",monospace; letter-spacing:.04em; }
        .endpoint-title { font-size:1.16rem; font-weight:650; }
        .endpoint-url { margin:8px 0 12px; overflow-wrap:anywhere; color:#d8c9b8; font:.9rem/1.6 "JetBrains Mono",monospace; }
        .endpoint-content { max-width:850px; color:var(--muted); font-size:1rem; }
        .endpoint-content h3,.endpoint-content h4 { margin:16px 0 8px; color:var(--ink); font-size:1rem; }
        .endpoint-content p { margin:8px 0; } .endpoint-content ul { margin:8px 0; padding-left:22px; }
        .endpoint-content li { margin:4px 0; } code,pre { font-family:"JetBrains Mono",monospace; }
        code { color:#e8c49f; font-size:.88em; }
        pre { margin:12px 0; padding:15px 17px; overflow:auto; border:1px solid #303a3c; border-radius:4px; background:var(--code); color:#e4e6e8; font-size:.86rem; line-height:1.6; }
        pre code { color:inherit; } .auth-box,.rate-limit-box { margin:12px 0; padding:10px 13px; border-left:2px solid var(--accent); background:var(--panel); color:var(--ink); }
        .error-box { margin:8px 0; color:var(--muted); }
        .api-table-wrap { margin:8px 0 16px; overflow-x:auto; }
        .api-table { width:100%; border-collapse:collapse; font-size:.92rem; line-height:1.5; }
        .api-table th,.api-table td { padding:8px 10px; border-bottom:1px solid var(--line); text-align:left; vertical-align:top; }
        .api-table th { color:var(--ink); background:var(--panel); font-size:.8rem; font-weight:600; letter-spacing:.025em; }
        .api-table td:first-child { width:150px; color:var(--ink); }
        .api-table td:nth-child(2) { width:190px; overflow-wrap:anywhere; }
        .api-sample { margin:8px 0 16px; }
        details.full-reference { margin-top:42px; border-top:1px solid var(--line); padding-top:18px; }
        details summary { width:max-content; cursor:pointer; color:var(--ink); font-weight:600; }
        footer { margin-top:46px; color:var(--muted); font-size:.86rem; }
        @media(max-width:760px) {
          .topbar-inner,main { width:min(100% - 36px,600px); } main { padding-top:40px; } .topbar-inner { min-height:60px; } .topbar nav { gap:14px; } .security { gap:10px 18px; }
          h1 { font-size:clamp(2.25rem,8vw,3rem); line-height:1.2; } .section-heading { margin:42px 0 14px; }
          .reference-layout { display:block; }
          .toc { z-index:5; display:flex; gap:6px; max-height:none; margin:0 calc((100vw - min(100vw - 36px,600px)) / -2); padding:8px 18px; overflow-x:auto; overflow-y:hidden; border-bottom:1px solid var(--line); background:var(--ground); scrollbar-width:none; }
          .toc::-webkit-scrollbar { display:none; }
          .toc a { flex:0 0 auto; min-height:44px; padding:8px 12px; border:0; border-radius:3px; white-space:nowrap; }
          .toc a[aria-current="location"] { background:var(--panel); color:var(--ink); }
          .endpoint-list { padding-top:8px; } .endpoint { scroll-margin-top:68px; }
        }
        @media(prefers-reduced-motion:reduce) { html { scroll-behavior:auto; } }
    </style>
</head>
<body>
    <div class="topbar"><div class="topbar-inner"><a class="brand" href="/">Counter API</a><nav aria-label="Main navigation"><a href="#endpoints">API reference</a><a href="/tools/curl">Command builder</a></nav></div></div>
    <main>
        <section class="hero" aria-labelledby="page-title"><h1 id="page-title">A small counter API. Built to be dependable.</h1>
        <p class="lead">Create counters, read their current values, and keep a durable record of each change.</p>
        <div class="hero-actions"><a class="primary-link" href="/tools/curl">Build setup commands <span aria-hidden="true">&nbsp;→</span></a><a href="#endpoints">Browse endpoints</a></div></section>
        <div class="security" aria-label="API guarantees"><span><strong>Scoped keys</strong> for tenant access</span><span><strong>Idempotent changes</strong> safe to retry</span><span><strong>Operation history</strong> records each mutation</span></div>
        <section id="endpoints" aria-labelledby="endpoint-heading"><div class="section-heading"><h2 id="endpoint-heading">API reference</h2></div>
        <div class="reference-layout">
        <nav class="toc" aria-label="Endpoints">
`)

	for _, req := range requests {
		sb.WriteString(fmt.Sprintf("<a href=\"#%s\">%s</a>\n", slugify(displayName(req)), html.EscapeString(displayName(req))))
	}

	sb.WriteString("</nav>\n<div class=\"endpoint-list\">\n")

	for _, req := range requests {
		sb.WriteString(fmt.Sprintf(`<article class="endpoint" id="%s">
            <div class="endpoint-header">
                <span class="method">%s</span>
                <h3 class="endpoint-title">%s</h3>
            </div>
            <div class="endpoint-url"><code>%s %s</code></div>
            <div class="endpoint-content">
%s
            </div>
        </article>
`,
			slugify(displayName(req)),
			req.HTTP.Method,
			html.EscapeString(displayName(req)),
			html.EscapeString(req.HTTP.Method),
			html.EscapeString(displayPath(req.HTTP.URL)),
			renderEndpointReference(req),
		))
	}

	sb.WriteString(`
        </div>
        </div>
    </section>
    <footer>Counter API · PostgreSQL-backed counters with durable operation history.</footer>
    </main>
    <script>
      (() => {
        const links = Array.from(document.querySelectorAll(".toc a"));
        const toc = document.querySelector(".toc");
        const sections = links.map((link) => document.querySelector(link.getAttribute("href"))).filter(Boolean);
        if (!("IntersectionObserver" in window) || !sections.length) return;
        const scrollBehavior = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";
        links.find((link) => link.hash === window.location.hash)?.setAttribute("aria-current", "location");
        const observer = new IntersectionObserver((entries) => {
          const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
          if (!visible.length) return;
          const activeId = visible[0].target.id;
          links.forEach((link) => {
            if (link.hash === "#" + activeId) {
              link.setAttribute("aria-current", "location");
              if (window.matchMedia("(max-width: 760px)").matches) {
                toc.scrollTo({ left: link.offsetLeft - (toc.clientWidth - link.clientWidth) / 2, behavior: scrollBehavior });
              } else {
                toc.scrollTo({ top: link.offsetTop - (toc.clientHeight - link.clientHeight) / 2, behavior: scrollBehavior });
              }
            } else {
              link.removeAttribute("aria-current");
            }
          });
        }, { rootMargin: "-18% 0px -68% 0px", threshold: 0 });
        sections.forEach((section) => observer.observe(section));
      })();
    </script>
</body>

</html>`)

	return sb.String()
}

type apiDocRow struct{ key, value, description string }

type endpointReference struct {
	requestHeaders, queryParameters, requestBody, responseHeaders []apiDocRow
	curl, responseJSON                                            string
}

func endpointReferenceFor(req BrunoRequest) endpointReference {
	jsonHeader := []apiDocRow{{"Content-Type", "application/json", "JSON response body."}}
	tenantKey := apiDocRow{"X-API-Key", "{{TENANT_API_KEY}}", "Tenant-scoped API key."}
	idem := "{{UUID_IDEMPOTENCY_KEY}}"
	refs := map[string]endpointReference{
		"Create Tenant": {
			requestHeaders: []apiDocRow{{"X-API-Key", "{{ADMIN_API_KEY}}", "Administrator key required."}, {"Content-Type", "application/json", "Request body is JSON."}},
			requestBody:    []apiDocRow{{"label", `"production"`, "Required and unique. 1–255 characters; letters, numbers, spaces, hyphens, underscores, and periods."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request POST "$BASE_URL/tenants" --header "X-API-Key: $ADMIN_API_KEY" --header "Content-Type: application/json" --data-raw '{"label":"production"}'`,
			responseJSON: `{"tenant_id":"01912345-6789-7000-8000-000000000001","label":"production","created_at":"2026-09-25T08:00:00Z","updated_at":"2026-09-25T08:00:00Z"}`,
		},
		"Create Credential": {
			requestHeaders: []apiDocRow{{"X-API-Key", "{{ADMIN_API_KEY}}", "Administrator key required to manage credentials."}, {"Content-Type", "application/json", "Request body is JSON."}},
			requestBody:    []apiDocRow{{"scopes", `["counter:read", "counter:increment", "counter:history"]`, "Optional; supports `tenant:read`, `counter:read`, `counter:list`, `counter:create`, `counter:increment`, `counter:adjust`, and `counter:history`. Defaults to read, increment, and history; listing is not granted by default."}, {"expires_at", `"2026-12-31T23:59:59Z"`, "Optional expiration timestamp; omitted for a non-expiring credential."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request POST "$BASE_URL/v2/tenants/$TENANT_ID/credentials" --header "X-API-Key: $ADMIN_API_KEY" --header "Content-Type: application/json" --data-raw '{"scopes":["counter:read","counter:increment","counter:history"]}'`,
			responseJSON: `{"credential_id":"01912345-6789-7000-8000-000000000010","tenant_id":"01912345-6789-7000-8000-000000000001","api_key":"ck_01912345-6789-7000-8000-000000000010.example-secret","scopes":["counter:read","counter:increment","counter:history"]}`,
		},
		"Create Counter": {
			requestHeaders: []apiDocRow{{"X-API-Key", "{{TENANT_API_KEY}}", "Administrator key or tenant key with `counter:create`."}, {"Content-Type", "application/json", "Request body is JSON."}},
			requestBody:    []apiDocRow{{"label", `"daily_steps"`, "Optional; unique within this tenant, 1–255 characters."}, {"initial_value", "0", "Optional int64; defaults to 0."}, {"max_delta", "50", "Optional positive int64; defaults to 50 and must be at least 1."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request POST "$BASE_URL/tenants/$TENANT_ID/counters" --header "X-API-Key: $TENANT_API_KEY" --header "Content-Type: application/json" --data-raw '{"label":"daily_steps","initial_value":0,"max_delta":50}'`,
			responseJSON: `{"counter_id":"01912345-6789-7000-8000-000000000002","tenant_id":"01912345-6789-7000-8000-000000000001","label":"daily_steps","value":0,"max_delta":50,"created_at":"2026-09-25T08:00:00Z","updated_at":"2026-09-25T08:00:00Z"}`,
		},
		"Get Counter": {
			requestHeaders: []apiDocRow{{tenantKey.key, tenantKey.value, "Tenant credentials require `counter:read`; administrator keys are also accepted."}}, responseHeaders: jsonHeader,
			curl: `curl --silent --show-error --request GET "$BASE_URL/v2/tenants/$TENANT_ID/counters/$COUNTER_ID" --header "X-API-Key: $TENANT_API_KEY"`, responseJSON: counterJSONSample,
		},
		"Increment Counter": {
			requestHeaders:  []apiDocRow{{tenantKey.key, tenantKey.value, "Requires `counter:increment`."}, {"Idempotency-Key", idem, "Required UUID; reuse only for retries of this same mutation."}},
			queryParameters: []apiDocRow{{"delta", "5", "Optional positive int64 increment; defaults to 1 and cannot exceed `max_delta`."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request POST "$BASE_URL/v2/tenants/$TENANT_ID/counters/$COUNTER_ID/inc?delta=5" --header "X-API-Key: $TENANT_API_KEY" --header "Idempotency-Key: $IDEMPOTENCY_KEY"`,
			responseJSON: `{"operation_id":"01912345-6789-7000-8000-000000000003","counter_id":"01912345-6789-7000-8000-000000000002","delta":5,"value":47,"replayed":false,"updated_at":"2026-09-25T12:01:00Z"}`,
		},
		"Set Counter": {
			requestHeaders: []apiDocRow{{tenantKey.key, tenantKey.value, "Requires the `counter:adjust` scope."}, {"Idempotency-Key", idem, "Required UUID; reuse only for retries of this same request."}, {"Content-Type", "application/json", "Request body is JSON."}},
			requestBody:    []apiDocRow{{"value", "100", "Required int64 value to set."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request POST "$BASE_URL/v2/tenants/$TENANT_ID/counters/$COUNTER_ID/set" --header "X-API-Key: $TENANT_API_KEY" --header "Idempotency-Key: $IDEMPOTENCY_KEY" --header "Content-Type: application/json" --data-raw '{"value":100}'`,
			responseJSON: `{"operation_id":"01912345-6789-7000-8000-000000000004","counter_id":"01912345-6789-7000-8000-000000000002","delta":53,"value":100,"replayed":false,"updated_at":"2026-09-25T12:03:00Z"}`,
		},
		"Operation History": {
			requestHeaders:  []apiDocRow{{tenantKey.key, tenantKey.value, "Requires the `counter:history` scope."}},
			queryParameters: []apiDocRow{{"limit", "50", "Optional integer from 1–100; defaults to 50."}, {"cursor", "{{CURSOR}}", "Optional opaque cursor; pass `next_cursor` unchanged."}}, responseHeaders: jsonHeader,
			curl:         `curl --silent --show-error --request GET "$BASE_URL/v2/tenants/$TENANT_ID/counters/$COUNTER_ID/operations?limit=50" --header "X-API-Key: $TENANT_API_KEY"`,
			responseJSON: `{"operations":[{"operation_id":"01912345-6789-7000-8000-000000000003","counter_id":"01912345-6789-7000-8000-000000000002","kind":"increment","delta":5,"value_before":42,"value_after":47,"metadata":{},"created_at":"2026-09-25T12:01:00Z","completed_at":"2026-09-25T12:01:00Z"}],"next_cursor":null}`,
		},
	}
	return refs[displayName(req)]
}

const counterJSONSample = `{"counter_id":"01912345-6789-7000-8000-000000000002","tenant_id":"01912345-6789-7000-8000-000000000001","label":"daily_steps","value":8421,"max_delta":100000,"created_at":"2026-09-25T08:00:00Z","updated_at":"2026-09-25T12:01:00Z"}`

func renderEndpointReference(req BrunoRequest) string {
	ref := endpointReferenceFor(req)
	var sb strings.Builder
	renderRows := func(title string, rows []apiDocRow) {
		if len(rows) == 0 {
			return
		}
		sb.WriteString("<h4>" + title + "</h4>\n<div class=\"api-table-wrap\"><table class=\"api-table\"><thead><tr><th scope=\"col\">Key</th><th scope=\"col\">Value</th><th scope=\"col\">Description</th></tr></thead><tbody>\n")
		for _, row := range rows {
			sb.WriteString("<tr><td><code>" + html.EscapeString(row.key) + "</code></td><td><code>" + html.EscapeString(row.value) + "</code></td><td>" + renderInlineMarkdown(row.description) + "</td></tr>\n")
		}
		sb.WriteString("</tbody></table></div>\n")
	}
	renderRows("Request headers", ref.requestHeaders)
	renderRows("Query parameters", ref.queryParameters)
	renderRows("Request body", ref.requestBody)
	renderRows("Response headers", ref.responseHeaders)
	if ref.curl != "" {
		sb.WriteString("<h4>Sample request</h4>\n<pre class=\"api-sample\"><code class=\"language-bash\">" + html.EscapeString(ref.curl) + "</code></pre>\n")
	}
	if ref.responseJSON != "" {
		var pretty bytes.Buffer
		if json.Indent(&pretty, []byte(ref.responseJSON), "", "  ") == nil {
			ref.responseJSON = pretty.String()
		}
		sb.WriteString("<h4>Sample response</h4>\n<pre class=\"api-sample\"><code class=\"language-json\">" + html.EscapeString(ref.responseJSON) + "</code></pre>\n")
	}
	sb.WriteString(renderMarkdown(homepageSupplementalDocs(req)))
	return sb.String()
}

func homepageSupplementalDocs(req BrunoRequest) string {
	lines := strings.Split(homepageDocs(req), "\n")
	var kept []string
	skipUntilHeading, skipCodeBlock := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			skipCodeBlock = !skipCodeBlock
			continue
		}
		if skipCodeBlock {
			continue
		}
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			skipUntilHeading = false
			heading := strings.ToLower(strings.TrimLeft(trimmed, "# "))
			switch heading {
			case "request", "request body", "response", "url parameters", "query parameters", "headers", "fields", "examples", "response (201 created)":
				skipUntilHeading = heading != "request" && heading != "response"
				continue
			}
		}
		if skipUntilHeading {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "docs/api-v2.md") || strings.Contains(lower, "canonical contract") || strings.Contains(lower, "v2 is the recommended contract") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func homepageRequests(all []BrunoRequest) []BrunoRequest {
	wanted := []string{"Create Tenant", "Create Credential", "Create Counter", "V2 Get Counter", "V2 Increment Counter", "V2 Set Counter", "V2 Operation History"}
	byName := make(map[string]BrunoRequest, len(all))
	for _, req := range all {
		byName[req.Info.Name] = req
	}
	if _, ok := byName["Create Credential"]; !ok {
		var req BrunoRequest
		req.Info.Name = "Create Credential"
		req.HTTP.Method = "POST"
		req.HTTP.URL = "{{BASE_URL}}/v2/tenants/{tenant_id}/credentials"
		req.Docs = "Creates a tenant-scoped key. Requires the administrator API key. The secret is returned once; store it securely. Supported scopes: `tenant:read`, `counter:read`, `counter:list`, `counter:create`, `counter:increment`, `counter:adjust`, and `counter:history`. By default, keys receive read, increment, and history scopes; listing, counter creation, and adjustment must be granted explicitly."
		byName[req.Info.Name] = req
	}
	selected := make([]BrunoRequest, 0, len(wanted))
	for _, name := range wanted {
		if req, ok := byName[name]; ok {
			selected = append(selected, req)
		}
	}
	return selected
}

func displayName(req BrunoRequest) string { return strings.TrimPrefix(req.Info.Name, "V2 ") }

func displayPath(raw string) string {
	path := strings.ReplaceAll(raw, "{{BASE_URL}}", "{base_url}")
	path = strings.ReplaceAll(path, "{{tenant_id}}", "{tenant_id}")
	path = strings.ReplaceAll(path, "{{counter_id}}", "{counter_id}")
	return path
}

func homepageDocs(req BrunoRequest) string {
	lines := strings.Split(req.Docs, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "# ") || strings.Contains(lower, "v1") || strings.Contains(lower, "v2") || strings.Contains(trimmed, "canonical contract") || strings.Contains(trimmed, "docs/api-v2.md") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
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
			result.WriteString(html.EscapeString(line) + "\n")
			continue
		}

		// Headers
		if strings.HasPrefix(trimmed, "### ") {
			closeList(&result, &inList)
			content := strings.TrimPrefix(trimmed, "### ")
			result.WriteString(fmt.Sprintf("<h4>%s</h4>\n", renderInlineMarkdown(content)))
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			closeList(&result, &inList)
			content := strings.TrimPrefix(trimmed, "# ")
			result.WriteString(fmt.Sprintf("<h3>%s</h3>\n", renderInlineMarkdown(content)))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			closeList(&result, &inList)
			content := strings.TrimPrefix(trimmed, "## ")
			if content == "Errors" {
				result.WriteString(fmt.Sprintf("<h3>%s</h3>\n", content))
				result.WriteString("<ul class=\"error-box\">\n")
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
	var result strings.Builder
	for len(text) > 0 {
		codeAt := strings.IndexByte(text, '`')
		boldAt := strings.Index(text, "**")
		markerAt, marker, tag := -1, "", ""
		if codeAt >= 0 {
			markerAt, marker, tag = codeAt, "`", "code"
		}
		if boldAt >= 0 && (markerAt < 0 || boldAt < markerAt) {
			markerAt, marker, tag = boldAt, "**", "strong"
		}
		if markerAt < 0 {
			result.WriteString(html.EscapeString(text))
			break
		}
		result.WriteString(html.EscapeString(text[:markerAt]))
		afterMarker := text[markerAt+len(marker):]
		endAt := strings.Index(afterMarker, marker)
		if endAt < 0 {
			result.WriteString(html.EscapeString(text[markerAt:]))
			break
		}
		result.WriteString("<" + tag + ">" + html.EscapeString(afterMarker[:endAt]) + "</" + tag + ">")
		text = afterMarker[endAt+len(marker):]
	}
	return result.String()
}

func closeList(sb *strings.Builder, inList *bool) {
	if *inList {
		sb.WriteString("</ul>\n")
		*inList = false
	}
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", "-"), "/", "-"))
}
