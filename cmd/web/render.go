package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

// renderer parses the embedded templates once and renders named templates or
// htmx partials against a data map.
type renderer struct {
	tmpl *template.Template
}

func newRenderer(fsys embed.FS) (*renderer, error) {
	funcs := template.FuncMap{
		"sensitive": isSensitiveKey,
	}
	t, err := template.New("").Funcs(funcs).ParseFS(fsys,
		"templates/*.html",
		"templates/partials/*.html",
	)
	if err != nil {
		return nil, err
	}
	return &renderer{tmpl: t}, nil
}

// page renders a full page template (which itself invokes "base").
func (r *renderer) page(w http.ResponseWriter, name string, data map[string]any) {
	r.renderTo(w, name, data)
}

// partial renders a named htmx fragment.
func (r *renderer) partial(w http.ResponseWriter, name string, data map[string]any) {
	r.renderTo(w, name, data)
}

func (r *renderer) renderTo(w http.ResponseWriter, name string, data map[string]any) {
	// Render into a buffer first so a template error doesn't emit a half page.
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, fmt.Sprintf("template %q: %v", name, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// toast writes a small htmx fragment used for action feedback. level is one of
// "ok", "warn", "error".
func (r *renderer) toast(w http.ResponseWriter, level, msg string) {
	r.partial(w, "toast", map[string]any{"Level": level, "Message": msg})
}

// data is a small helper to build template data maps with the common fields
// every page needs (active nav item, etc.).
func data(active string, kv ...any) map[string]any {
	m := map[string]any{"Active": active}
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		m[key] = kv[i+1]
	}
	return m
}
