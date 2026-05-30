package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/example/dddcqrs/internal/app/bus"
	"github.com/example/dddcqrs/internal/app/command"
	"github.com/example/dddcqrs/internal/app/query"
	"github.com/example/dddcqrs/internal/interfaces/http/gen"
)

func (s *Server) RegisterUser(w http.ResponseWriter, r *http.Request) {
	var body gen.RegisterUserJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash error")
		return
	}
	res, err := bus.Dispatch[command.RegisterUserResult](r.Context(), s.reg, command.RegisterUser{
		Email:        string(body.Email),
		PasswordHash: string(hash),
	})
	if errors.Is(err, command.ErrEmailTaken) {
		writeErr(w, http.StatusConflict, "email taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gen.IDResponse{Id: res.UserID})
}

func (s *Server) ActivateUser(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	if _, err := bus.Dispatch[command.ActivateUserResult](r.Context(), s.reg, command.ActivateUser{UserID: id}); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetUser(w http.ResponseWriter, r *http.Request, id gen.PathID) {
	view, err := bus.Ask[query.UserView](r.Context(), s.reg, query.GetUser{UserID: id})
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request, params gen.ListUsersParams) {
	limit := int32(20)
	if params.Limit != nil {
		limit = int32(*params.Limit)
	}
	views, err := bus.Ask[[]query.UserView](r.Context(), s.reg, query.ListUsers{Limit: limit})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error")
		return
	}
	writeJSON(w, http.StatusOK, views)
}
