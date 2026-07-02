package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"egeism/internal/domain"
)

// notificationsResp wraps the feed with the exact unread count for the bell
// badge — the item list is limit-bounded, so the client can't just count it.
type notificationsResp struct {
	Unread int64                 `json:"unread"`
	Items  []domain.Notification `json:"items"`
}

// handleListNotifications returns the acting user's in-app notifications
// (newest first) plus the unread count. The web polls this for the bell.
func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	limit := queryInt(r, "limit", 30)
	if limit > 100 {
		limit = 100
	}
	items, err := s.store.ListNotifications(r.Context(), user.ID, limit)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	unread, err := s.store.CountUnreadNotifications(r.Context(), user.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notificationsResp{Unread: unread, Items: items})
}

// handleMarkNotificationRead stamps one of the acting user's notifications
// read (e.g. when they click through to the test).
func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "notificationID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if err := s.store.MarkNotificationRead(r.Context(), id, user.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// handleMarkAllNotificationsRead clears the acting user's unread badge.
func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	if err := s.store.MarkAllNotificationsRead(r.Context(), user.ID); err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
