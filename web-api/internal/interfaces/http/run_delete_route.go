package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/example/dddcqrs/internal/app/bus"
	pipecmd "github.com/example/dddcqrs/internal/app/command/pipeline"
)

// MountRunDelete attaches DELETE /api/runs/{id}. Kept in its own file so we
// don't have to wait on oapi-codegen to grow a Delete operation in the
// generated chi router.
func (s *Server) MountRunDelete(r chi.Router) {
	r.Delete("/api/runs/{id}", s.DeleteRun)
}

func (s *Server) DeleteRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := bus.Dispatch[pipecmd.DeleteRunResult](r.Context(), s.reg, pipecmd.DeleteRun{RunID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
