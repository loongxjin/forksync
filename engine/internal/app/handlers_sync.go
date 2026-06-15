package app

import (
	"fmt"
	"net/http"

	"github.com/loongxjin/forksync/engine/pkg/types"
)

// registerSyncRoutes wires the sync endpoints.
func (s *Server) registerSyncRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sync/all", s.handleSyncAll)
	mux.HandleFunc("POST /sync/repos/{name}", s.handleSyncRepo)
}

func (s *Server) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	results := s.deps.Syncer.SyncAll(r.Context())
	syncResults := make([]types.SyncResult, 0, len(results))
	for _, res := range results {
		syncResults = append(syncResults, res.ToSyncResult())
	}
	writeOK(w, types.SyncData{Results: syncResults})
}

func (s *Server) handleSyncRepo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	r2, ok := s.deps.Store.GetByName(name)
	if !ok {
		writeErr[types.SyncData](w, fmt.Errorf("repository %q not found", name))
		return
	}

	result := s.deps.Syncer.SyncRepo(r.Context(), r2)
	writeOK(w, types.SyncData{Results: []types.SyncResult{result.ToSyncResult()}})
}
