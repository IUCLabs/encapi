package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/IUCLabs/encapi/internal/database"
	"github.com/IUCLabs/encapi/internal/enc"
	"github.com/IUCLabs/encapi/pkg/models"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeYAML(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// mapErr translates store errors into HTTP status codes.
func mapErr(w http.ResponseWriter, err error) {
	if errors.Is(err, database.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// ---------- ENC ----------

// resolveNode loads a node, its role, and any distro params.
func (s *Server) resolveNode(w http.ResponseWriter, r *http.Request) (models.Node, models.Role, map[string]any, bool) {
	certname := chi.URLParam(r, "certname")
	node, err := s.store.GetNode(r.Context(), certname)
	if err != nil {
		mapErr(w, err)
		return models.Node{}, models.Role{}, nil, false
	}
	role, err := s.store.GetRole(r.Context(), node.Role)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			// A node pinned to a role that no longer exists: still classify it,
			// just without default params, rather than 500.
			role = &models.Role{Name: node.Role}
		} else {
			mapErr(w, err)
			return models.Node{}, models.Role{}, nil, false
		}
	}
	distroParams, err := s.resolver.Resolve(r.Context(), certname)
	if err != nil {
		writeError(w, http.StatusBadGateway, "distro resolver: "+err.Error())
		return models.Node{}, models.Role{}, nil, false
	}
	return *node, *role, distroParams, true
}

func (s *Server) handleENCFinal(w http.ResponseWriter, r *http.Request) {
	node, role, distroParams, ok := s.resolveNode(w, r)
	if !ok {
		return
	}
	out, err := enc.Final(node, role, distroParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeYAML(w, out)
}

func (s *Server) handleENCCobbler(w http.ResponseWriter, r *http.Request) {
	node, role, distroParams, ok := s.resolveNode(w, r)
	if !ok {
		return
	}
	out, err := enc.Cobbler(node, role, distroParams)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeYAML(w, out)
}

// ---------- roles ----------

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.store.ListRoles(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) getRole(w http.ResponseWriter, r *http.Request) {
	role, err := s.store.GetRole(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) putRole(w http.ResponseWriter, r *http.Request) {
	var role models.Role
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	role.Name = chi.URLParam(r, "name")
	if err := s.store.UpsertRole(r.Context(), &role); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteRole(r.Context(), chi.URLParam(r, "name")); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- statuses ----------

func (s *Server) listStatuses(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.store.ListStatuses(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.store.GetStatus(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) putStatus(w http.ResponseWriter, r *http.Request) {
	var status models.Status
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	status.Name = chi.URLParam(r, "name")
	if err := s.store.UpsertStatus(r.Context(), &status); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) deleteStatus(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteStatus(r.Context(), chi.URLParam(r, "name")); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- nodes ----------

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListNodes(r.Context())
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	node, err := s.store.GetNode(r.Context(), chi.URLParam(r, "certname"))
	if err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) putNode(w http.ResponseWriter, r *http.Request) {
	var node models.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	node.Certname = chi.URLParam(r, "certname")
	if node.Role == "" || node.Environment == "" {
		writeError(w, http.StatusBadRequest, "role and environment are required")
		return
	}
	if err := s.store.UpsertNode(r.Context(), &node); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// putNodeEnvironment changes only a node's environment. Testing-scoped tokens
// may only move a node to "testing".
func (s *Server) putNodeEnvironment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Environment == "" {
		writeError(w, http.StatusBadRequest, "environment is required")
		return
	}
	if r.Context().Value(scopeKey{}) != scopeFull && body.Environment != "testing" {
		writeError(w, http.StatusForbidden, "token may only set environment to testing")
		return
	}
	node, err := s.store.GetNode(r.Context(), chi.URLParam(r, "certname"))
	if err != nil {
		mapErr(w, err)
		return
	}
	node.Environment = body.Environment
	if err := s.store.UpsertNode(r.Context(), node); err != nil {
		mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNode(r.Context(), chi.URLParam(r, "certname")); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
