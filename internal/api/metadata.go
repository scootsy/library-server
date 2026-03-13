package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/scootsy/library-server/internal/database/queries"
	"github.com/scootsy/library-server/internal/metadata"
)

// MetadataHandler handles REST endpoints for metadata operations.
type MetadataHandler struct {
	db     *sql.DB
	engine *metadata.Engine
}

// Refresh triggers a metadata refresh for a specific work.
func (h *MetadataHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")

	work, err := queries.GetWorkByID(h.db, workID)
	if err != nil {
		slog.Error("failed to get work", "id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get work")
		return
	}
	if work == nil {
		writeError(w, http.StatusNotFound, "work not found")
		return
	}

	if err := h.engine.EnqueueWork(workID, "refresh", 1); err != nil {
		slog.Error("failed to enqueue refresh", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to enqueue refresh")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "queued",
		"work_id": workID,
	})
}

// GetTasks returns all metadata tasks for a given work.
func (h *MetadataHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	workID := r.PathValue("workID")

	tasks, err := queries.GetTasksForWork(h.db, workID)
	if err != nil {
		slog.Error("failed to get tasks", "work_id", workID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": tasks})
}

type applyCandidateRequest struct {
	CandidateIndex int `json:"candidate_index"`
}

// ApplyCandidate applies a selected metadata candidate from a task.
func (h *MetadataHandler) ApplyCandidate(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskID")

	var req applyCandidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	task, err := queries.GetMetadataTaskByID(h.db, taskID)
	if err != nil {
		slog.Error("failed to get task", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	// Parse candidates from JSON.
	var candidates []metadata.ScoredCandidate
	if task.Candidates != "" {
		if err := json.Unmarshal([]byte(task.Candidates), &candidates); err != nil {
			slog.Error("failed to parse candidates", "task_id", taskID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to parse candidates")
			return
		}
	}

	if req.CandidateIndex < 0 || req.CandidateIndex >= len(candidates) {
		writeError(w, http.StatusBadRequest, "candidate_index out of range")
		return
	}

	selected := candidates[req.CandidateIndex]
	if err := h.engine.ApplyCandidate(task.WorkID, selected); err != nil {
		slog.Error("failed to apply candidate", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to apply candidate")
		return
	}

	if err := queries.SetTaskSelected(h.db, taskID, req.CandidateIndex); err != nil {
		slog.Error("failed to record selection", "task_id", taskID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "applied",
		"task_id": taskID,
	})
}

// ReviewQueue returns works that need metadata review.
func (h *MetadataHandler) ReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r, "limit", 50)
	offset := parseIntParam(r, "offset", 0)

	works, total, err := queries.GetReviewQueue(h.db, limit, offset)
	if err != nil {
		slog.Error("failed to get review queue", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get review queue")
		return
	}

	writeJSON(w, http.StatusOK, paginatedResponse{
		Data:   works,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}
