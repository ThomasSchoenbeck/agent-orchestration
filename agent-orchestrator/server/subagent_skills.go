package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Subagent skills (spawnable units of work — Subagents feature)
// =========================================================================

func (s *Server) handleSubagentSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := s.db.ListSubagentSkills(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if skills == nil {
			skills = []*db.SubagentSkill{}
		}
		api.WriteJSON(w, http.StatusOK, skills)

	case http.MethodPost:
		var sk db.SubagentSkill
		if !s.decodeJSON(w, r, &sk) {
			return
		}
		if sk.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		sk.Enabled = true
		if err := s.db.CreateSubagentSkill(r.Context(), &sk); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, sk)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleSubagentSkillDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/subagent-skills/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	// POST /api/subagent-skills/seed — import the starter set (idempotent).
	if id == "seed" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		n, err := s.db.SeedSubagentSkills(r.Context(), db.DefaultSubagentSkills())
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]int{"seeded": n})
		return
	}

	switch r.Method {
	case http.MethodGet:
		sk, err := s.db.GetSubagentSkill(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, sk)

	case http.MethodPut:
		sk, err := s.db.GetSubagentSkill(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if !s.decodeJSON(w, r, sk) {
			return
		}
		sk.ID = id
		if err := s.db.UpdateSubagentSkill(r.Context(), sk); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, sk)

	case http.MethodDelete:
		if err := s.db.DeleteSubagentSkill(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
