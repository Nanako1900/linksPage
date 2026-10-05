package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/Nanako1900/linksPage/internal/site"
)

// OpenAPIPath serves the generated OpenAPI 3.1 document.
const OpenAPIPath = "/api/openapi.json"

// SnapshotSource provides the current public snapshot.
type SnapshotSource interface {
	Current() *site.Snapshot
}

// humaConfig returns the API configuration. Docs UI (which would load
// third-party scripts) and $schema links are disabled; the spec is served
// by our own handler.
func humaConfig(version string) huma.Config {
	cfg := huma.DefaultConfig("LinksPage API", version)
	cfg.OpenAPIPath = ""
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil
	cfg.Transformers = []huma.Transformer{problemTransformer}
	cfg.Info.Description = "LinksPage public and admin API. Errors use RFC 9457 problem+json with `code` and `requestId`."
	return cfg
}

// BootstrapOutput is the response of GET /api/v1/public/bootstrap.
type BootstrapOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Data site.Bootstrap `json:"data"`
	}
}

// publicAPI holds dependencies of public operations.
type publicAPI struct {
	snapshots SnapshotSource
	ready     *Readiness
}

func (p publicAPI) bootstrap(_ context.Context, _ *struct{}) (*BootstrapOutput, error) {
	if !p.ready.IsReady() {
		return nil, NewProblem(http.StatusServiceUnavailable, CodeNotReady, "the service is starting")
	}
	out := &BootstrapOutput{CacheControl: "no-cache"}
	out.Body.Data = p.snapshots.Current().Bootstrap()
	return out, nil
}

// registerOperations registers every huma operation on api.
func registerOperations(api huma.API, p publicAPI) {
	huma.Register(api, huma.Operation{
		OperationID: "getPublicBootstrap",
		Method:      http.MethodGet,
		Path:        "/api/v1/public/bootstrap",
		Summary:     "Public bootstrap data",
		Description: "Site settings and the default page. The same document is embedded in HTML as #lp-data.",
		Tags:        []string{"public"},
		Errors:      []int{http.StatusServiceUnavailable},
	}, p.bootstrap)
}

// newAPI mounts huma on r and registers all operations.
func newAPI(r chi.Router, version string, p publicAPI) huma.API {
	api := humachi.New(r, humaConfig(version))
	registerOperations(api, p)
	return api
}

// OpenAPISpec returns the OpenAPI 3.1 document as indented JSON without
// needing any runtime dependency (used by `linkspage openapi`).
func OpenAPISpec(version string) ([]byte, error) {
	api := newAPI(chi.NewRouter(), version, publicAPI{})
	b, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal openapi: %w", err)
	}
	return append(b, '\n'), nil
}

// openAPIHandler serves the spec, marshaled once.
func openAPIHandler(api huma.API) http.HandlerFunc {
	var (
		once sync.Once
		spec []byte
		err  error
	)
	return func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { spec, err = json.Marshal(api.OpenAPI()) })
		if err != nil {
			loggerFrom(r.Context()).Error("marshal openapi", "error", err)
			writeProblem(w, r, http.StatusInternalServerError, CodeInternal, "")
			return
		}
		w.Header().Set("Content-Type", "application/openapi+json")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(spec)
	}
}
