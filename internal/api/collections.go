package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/scootsy/library-server/internal/database/queries"
)

// CollectionsHandler handles REST endpoints for collections.
type CollectionsHandler struct {
	db *sql.DB
}

// List returns all collections.
func (h *CollectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	collections, err := queries.ListCollections(h.db, userID)
	if err != nil {
		slog.Error("failed to list collections", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": collections})
}

// Get returns a collection with its works.
func (h *CollectionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	coll, err := queries.GetCollectionByID(h.db, id)
	if err != nil {
		slog.Error("failed to get collection", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get collection")
		return
	}
	if coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}

	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetCollectionWorks(h.db, id, limit, offset)
	if err != nil {
		slog.Error("failed to get collection works", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get works")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"collection": coll,
		"works":      paginatedResponse{Data: works, Total: total, Limit: limit, Offset: offset},
	})
}

type createCollectionRequest struct {
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	CollectionType string `json:"collection_type"`
	SmartFilter    string `json:"smart_filter"`
	DeviceID       string `json:"device_id"`
	IsPublic       bool   `json:"is_public"`
	SortOrder      int    `json:"sort_order"`
}

// Create creates a new collection.
func (h *CollectionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.CollectionType == "" {
		req.CollectionType = "manual"
	}
	// Until auth is implemented (Phase 4), use a placeholder user ID if none provided.
	if req.UserID == "" {
		req.UserID = "system"
	}

	coll := &queries.Collection{
		ID:             uuid.NewString(),
		UserID:         req.UserID,
		Name:           req.Name,
		Description:    req.Description,
		CollectionType: req.CollectionType,
		SmartFilter:    req.SmartFilter,
		DeviceID:       req.DeviceID,
		IsPublic:       req.IsPublic,
		SortOrder:      req.SortOrder,
	}

	if err := queries.CreateCollection(h.db, coll); err != nil {
		slog.Error("failed to create collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create collection")
		return
	}

	writeJSON(w, http.StatusCreated, coll)
}

// Update modifies an existing collection.
func (h *CollectionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	existing, err := queries.GetCollectionByID(h.db, id)
	if err != nil {
		slog.Error("failed to get collection", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get collection")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}

	var req createCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	existing.Description = req.Description
	if req.CollectionType != "" {
		existing.CollectionType = req.CollectionType
	}
	existing.SmartFilter = req.SmartFilter
	existing.DeviceID = req.DeviceID
	existing.IsPublic = req.IsPublic
	existing.SortOrder = req.SortOrder

	if err := queries.UpdateCollection(h.db, existing); err != nil {
		slog.Error("failed to update collection", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update collection")
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

// Delete removes a collection.
func (h *CollectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := queries.DeleteCollection(h.db, id); err != nil {
		slog.Error("failed to delete collection", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete collection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type addWorkRequest struct {
	WorkID   string `json:"work_id"`
	Position int    `json:"position"`
}

// AddWork adds a work to a collection.
func (h *CollectionsHandler) AddWork(w http.ResponseWriter, r *http.Request) {
	collID := r.PathValue("id")

	var req addWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.WorkID == "" {
		writeError(w, http.StatusBadRequest, "work_id is required")
		return
	}

	if err := queries.AddWorkToCollection(h.db, collID, req.WorkID, req.Position); err != nil {
		slog.Error("failed to add work to collection", "collection_id", collID, "work_id", req.WorkID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to add work")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveWork removes a work from a collection.
func (h *CollectionsHandler) RemoveWork(w http.ResponseWriter, r *http.Request) {
	collID := r.PathValue("id")
	workID := r.PathValue("workID")

	if err := queries.RemoveWorkFromCollection(h.db, collID, workID); err != nil {
		slog.Error("failed to remove work from collection", "collection_id", collID, "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove work")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
