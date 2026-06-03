package server

import (
	"net/http"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// =========================================================================
// Skills (skill definitions — Feature 6)
// =========================================================================

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := s.db.ListSkillDefinitions(r.Context())
		if err != nil {
			s.internalError(w, err)
			return
		}
		if skills == nil {
			skills = []*db.SkillDefinition{}
		}
		api.WriteJSON(w, http.StatusOK, skills)

	case http.MethodPost:
		var sd db.SkillDefinition
		if !s.decodeJSON(w, r, &sd) {
			return
		}
		if sd.Name == "" {
			api.WriteError(w, http.StatusBadRequest, api.ErrCodeInvalidInput, "name is required")
			return
		}
		sd.Enabled = true
		if err := s.db.CreateSkillDefinition(r.Context(), &sd); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, sd)

	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleSkillDetail(w http.ResponseWriter, r *http.Request) {
	id := pathSegment(r.URL.Path, "/api/skills/", 0)
	if id == "" {
		http.NotFound(w, r)
		return
	}

	// POST /api/skills/seed — import the starter skill set (idempotent).
	if id == "seed" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		n, err := s.db.SeedSkillDefinitions(r.Context(), db.DefaultSkillDefinitions())
		if err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]int{"seeded": n})
		return
	}

	switch r.Method {
	case http.MethodGet:
		sd, err := s.db.GetSkillDefinition(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, sd)

	case http.MethodPut:
		sd, err := s.db.GetSkillDefinition(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		if !s.decodeJSON(w, r, sd) {
			return
		}
		sd.ID = id
		if err := s.db.UpdateSkillDefinition(r.Context(), sd); err != nil {
			s.internalError(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, sd)

	case http.MethodDelete:
		if err := s.db.DeleteSkillDefinition(r.Context(), id); err != nil {
			s.internalError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}
