package server

import (
	"net/http"
	"strings"

	"agent-orchestrator/api"
	"agent-orchestrator/db"
)

// handleSettings handles GET /api/settings
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	settings, err := s.db.ListSettings(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	if settings == nil {
		settings = []*db.Setting{}
	}
	api.WriteJSON(w, http.StatusOK, settings)
}

// handleSettingDetail handles PUT /api/settings/:key
func (s *Server) handleSettingDetail(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/settings/")
	if key == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		setting, err := s.db.GetSetting(r.Context(), key)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.ErrCodeNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, setting)
		return
	case http.MethodPut:
		// handled below
	default:
		methodNotAllowed(w)
		return
	}

	var body struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if !s.decodeJSON(w, r, &body) {
		return
	}

	if err := s.db.SetSetting(r.Context(), key, body.Value, body.Description); err != nil {
		s.internalError(w, err)
		return
	}

	setting, err := s.db.GetSetting(r.Context(), key)
	if err != nil {
		s.internalError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, setting)
}
