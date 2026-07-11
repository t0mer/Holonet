package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(openAPISpec)
}

// docsHTML is a self-contained API docs page. It deliberately loads no external
// scripts (no CDN dependency / SRI risk): the machine-readable OpenAPI 3 spec is
// the source of truth at /api/openapi.yaml, consumable by any OpenAPI viewer.
const docsHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>HoloNet API</title>
    <style>
      body { font-family: system-ui, sans-serif; max-width: 46rem; margin: 3rem auto; padding: 0 1rem; line-height: 1.55; }
      code { background: #f2f2f2; padding: .1rem .3rem; border-radius: .2rem; }
      a { color: #2563eb; }
      pre { background: #f7f7f7; padding: 1rem; border-radius: .4rem; overflow-x: auto; }
    </style>
  </head>
  <body>
    <h1>HoloNet API</h1>
    <p>The machine-readable OpenAPI 3 specification is served at
      <a href="/api/openapi.yaml">/api/openapi.yaml</a>. Open it in your editor
      or any OpenAPI viewer (Swagger Editor, Redoc, Postman, an IDE plugin) to
      explore and try the endpoints.</p>
    <h2>Authentication</h2>
    <p>Session cookie <code>holonet_session</code> from
      <code>POST /api/v1/auth/login</code>. First run:
      <code>POST /api/v1/auth/setup</code>. When <code>auth.enabled</code> is
      <code>false</code> the API is open (e.g. behind Cloudflare Access).</p>
    <p>Sealed secrets are write-only and never returned by the API.</p>
  </body>
</html>`

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(docsHTML))
}
