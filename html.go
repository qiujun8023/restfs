package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldhtml "github.com/yuin/goldmark/renderer/html"
)

// renderMarkdown 将 Markdown 转换为 HTML（保留原始 HTML 内容）
func renderMarkdown(src []byte) string {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(goldhtml.WithUnsafe()),
	)
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return ""
	}
	return buf.String()
}

type breadcrumbPart struct {
	Name   string
	Href   string
	IsLast bool
}

// splitBreadcrumb 将路径 "/a/b/c/" 拆分为面包屑节点列表
func splitBreadcrumb(path string) []breadcrumbPart {
	var parts []breadcrumbPart

	if path == "/" {
		return []breadcrumbPart{{Name: "~", Href: "/", IsLast: true}}
	}

	parts = append(parts, breadcrumbPart{Name: "~", Href: "/", IsLast: false})

	trimmed := strings.Trim(path, "/")
	segments := strings.Split(trimmed, "/")

	href := "/"
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		href += url.PathEscape(seg) + "/"
		parts = append(parts, breadcrumbPart{
			Name:   seg,
			Href:   href,
			IsLast: i == len(segments)-1,
		})
	}
	return parts
}

var dirTmpl = template.Must(template.New("dir").Funcs(template.FuncMap{
	"formatSize":      formatSize,
	"safeHTML":        func(s string) template.HTML { return template.HTML(s) },
	"splitBreadcrumb": splitBreadcrumb,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Index of {{.Path}}</title>
  <style>
    :root {
      --bg: #f9fafb;
      --card-bg: #ffffff;
      --text-main: #111827;
      --text-muted: #6b7280;
      --border: #e5e7eb;
      --primary: #2563eb;
      --row-hover: #f3f4f6;
      --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
      --shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
    }
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      background-color: var(--bg);
      color: var(--text-main);
      line-height: 1.5;
      padding: 48px 24px;
      -webkit-font-smoothing: antialiased;
    }
    .container { max-width: 960px; margin: 0 auto; }
    
    .breadcrumb {
      display: flex; align-items: center; flex-wrap: wrap;
      font-size: 20px; font-weight: 600; margin-bottom: 24px;
      color: var(--text-main); padding: 0 4px; letter-spacing: -0.01em;
    }
    .breadcrumb a { color: var(--text-muted); text-decoration: none; transition: color 0.2s; }
    .breadcrumb a:hover { color: var(--primary); }
    .breadcrumb .sep { color: #d1d5db; margin: 0 10px; font-weight: 400; user-select: none; }
    .breadcrumb .current { color: var(--text-main); }

    .card {
      background: var(--card-bg); border-radius: 12px;
      box-shadow: var(--shadow);
      border: 1px solid var(--border); overflow: hidden;
    }
    .table-wrapper {
      width: 100%;
      overflow-x: auto;
    }
    table { width: 100%; border-collapse: collapse; text-align: left; }
    th, td { padding: 14px 24px; white-space: nowrap; }
    th {
      background-color: #f8fafc; font-size: 13px; font-weight: 600;
      color: var(--text-muted);
      border-bottom: 1px solid var(--border);
    }
    th.right, td.right { text-align: right; }
    tr { border-bottom: 1px solid var(--border); transition: background-color 0.2s; }
    tr:last-child { border-bottom: none; }
    tr:hover { background-color: var(--row-hover); }
    
    .name-cell { display: flex; align-items: center; gap: 12px; }
    .icon { font-size: 20px; width: 24px; text-align: center; color: var(--text-muted); }
    .name-link { color: var(--text-main); text-decoration: none; font-weight: 500; transition: color 0.2s; }
    .name-link:hover { color: var(--primary); }
    .dir-link { color: var(--primary); font-weight: 600; }
    .size, .mtime { color: var(--text-muted); font-size: 14px; font-variant-numeric: tabular-nums; }
    .empty { text-align: center; padding: 48px; color: var(--text-muted); }

    .readme-card { margin-top: 32px; padding-bottom: 0; }
    .readme-header {
      padding: 12px 24px; background-color: #f8fafc;
      border-bottom: 1px solid var(--border); font-weight: 600;
      font-size: 14px; color: var(--text-muted); display: flex;
      align-items: center; gap: 8px;
    }
    .readme-body { padding: 32px 40px; color: #374151; font-size: 15px; line-height: 1.7; overflow-wrap: break-word; }
    .readme-body h1, .readme-body h2, .readme-body h3, .readme-body h4 { color: #111827; font-weight: 700; margin: 1.5em 0 0.75em; }
    .readme-body h1 { font-size: 2.2em; border-bottom: 1px solid var(--border); padding-bottom: 0.3em; margin-top: 0; }
    .readme-body h2 { font-size: 1.7em; border-bottom: 1px solid var(--border); padding-bottom: 0.3em; }
    .readme-body p { margin-bottom: 1.2em; }
    .readme-body a { color: var(--primary); text-decoration: none; }
    .readme-body a:hover { text-decoration: underline; }
    .readme-body code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; background: #f1f5f9; padding: 0.2em 0.4em; border-radius: 4px; font-size: 0.9em; }
    .readme-body pre { background: #1e293b; color: #f8fafc; padding: 16px 20px; border-radius: 8px; overflow-x: auto; margin-bottom: 1.2em; }
    .readme-body pre code { background: transparent; padding: 0; color: inherit; }
    .readme-body ul, .readme-body ol { padding-left: 1.5em; margin-bottom: 1.2em; }
    .readme-body blockquote { border-left: 4px solid #cbd5e1; padding-left: 1em; color: var(--text-muted); font-style: italic; margin-bottom: 1.2em; }
    .readme-body img { max-width: 100%; height: auto; border-radius: 6px; }
    .readme-body table { border-collapse: collapse; margin-bottom: 1.2em; width: 100%; }
    .readme-body th, .readme-body td { border: 1px solid var(--border); padding: 8px 12px; }
    .readme-body th { background: #f8fafc; }

    /* Responsive adjustments */
    @media (max-width: 768px) {
      body { padding: 24px 16px; }
      th, td { padding: 12px 16px; }
      .breadcrumb { font-size: 18px; margin-bottom: 16px; }
      .readme-body { padding: 24px 20px; }
      .readme-body h1 { font-size: 1.8em; }
      .readme-body h2 { font-size: 1.5em; }
    }
    @media (max-width: 640px) {
      body { padding: 16px 12px; }
      .breadcrumb { font-size: 16px; margin-bottom: 12px; }
      th, td { padding: 12px; }
      .hide-mobile { display: none; }
      .readme-body { padding: 16px 12px; }
      .icon { font-size: 18px; width: 20px; }
      .name-cell { gap: 8px; }
    }
  </style>
</head>
<body>
  <div class="container">
    <nav class="breadcrumb">
      {{range $i, $p := splitBreadcrumb .Path}}
        {{if .IsLast}}
          <span class="current">{{if eq $i 0}}~{{else}}{{.Name}}{{end}}</span>
        {{else}}
          <a href="{{.Href}}">{{if eq $i 0}}~{{else}}{{.Name}}{{end}}</a>
          <span class="sep">/</span>
        {{end}}
      {{end}}
    </nav>

    <div class="card">
      <div class="table-wrapper">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th class="right">Size</th>
              <th class="right hide-mobile">Modified (UTC)</th>
            </tr>
          </thead>
          <tbody>
            {{if ne .Path "/"}}
            <tr>
              <td><div class="name-cell"><span class="icon">↩</span><a class="name-link dir-link" href="../">..</a></div></td>
              <td></td><td class="hide-mobile"></td>
            </tr>
            {{end}}
            {{range .Entries}}
            <tr>
              <td>
                <div class="name-cell">
                  {{if eq .Type "directory"}}
                    <span class="icon">📁</span>
                    <a class="name-link dir-link" href="{{.Path}}">{{.Name}}</a>
                  {{else}}
                    <span class="icon">📄</span>
                    <a class="name-link" href="{{.Path}}">{{.Name}}</a>
                  {{end}}
                </div>
              </td>
              <td class="size right">{{if eq .Type "directory"}}—{{else}}{{formatSize .Size}}{{end}}</td>
              <td class="mtime right hide-mobile">{{.Modified}}</td>
            </tr>
            {{end}}
            {{if eq (len .Entries) 0}}
            <tr><td colspan="3" class="empty">Empty directory</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </div>

    {{if .ReadmeHTML}}
    <div class="card readme-card">
      <div class="readme-header"><span>📄</span> {{.ReadmeName}}</div>
      <div class="readme-body">{{safeHTML .ReadmeHTML}}</div>
    </div>
    {{end}}
  </div>
</body>
</html>
`))

type dirTemplateData struct {
	Path       string
	Entries    []dirEntry
	ReadmeName string
	ReadmeHTML string
}

func renderDirHTML(w http.ResponseWriter, path string, entries []dirEntry, readmeName, readmeHTML string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dirTmpl.Execute(w, dirTemplateData{
		Path:       path,
		Entries:    entries,
		ReadmeName: readmeName,
		ReadmeHTML: readmeHTML,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
