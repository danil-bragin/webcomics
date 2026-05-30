package http

import (
	"net/http"

	"github.com/example/dddcqrs/internal/domain/formats"
)

// ListFormats returns the built-in format catalog. Custom user formats will
// land here too once the editor lands; for now system-only.
func (s *Server) ListFormats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, formats.System())
}
