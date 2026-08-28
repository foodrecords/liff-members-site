package member

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/presenter"
)

const deletionGracePeriod = 30 * 24 * time.Hour

type deletionRequest struct {
	Confirmation string `json:"confirmation" validate:"required"`
}

type deletionResponse struct {
	DeletedAt string `json:"deleted_at"`
	PurgeAt   string `json:"purge_at"`
}

func authenticatedMemberRef(h handler, r *http.Request) (*firestore.DocumentRef, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	profile, err := getProfile(token)
	if err != nil {
		return nil, err
	}
	return config.DataCollection(h.fs, "members").Doc(profile.UserID), nil
}

func (h handler) ScheduleDeletion(w http.ResponseWriter, r *http.Request) {
	var body deletionRequest
	// 共通interpreterはDELETEをquery parameterとして扱うため、
	// この確認値だけはJSON bodyから明示的に読み取る。
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Confirmation != "DELETE" {
		presenter.BadRequest(w, "ACCOUNT_DELETE_CONFIRMATION_REQUIRED")
		return
	}
	ref, err := authenticatedMemberRef(h, r)
	if err != nil {
		presenter.Forbidden(w, "INVALID_TOKEN")
		return
	}
	now := time.Now().UTC()
	purgeAt := now.Add(deletionGracePeriod)
	if _, err := ref.Update(r.Context(), []firestore.Update{
		{Path: "deleted_at", Value: now},
		{Path: "purge_at", Value: purgeAt},
		{Path: "deletion_requested_at", Value: firestore.Delete},
		{Path: "deletion_scheduled_at", Value: firestore.Delete},
	}); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.EncodeWithMessage(w, deletionResponse{DeletedAt: now.Format(time.RFC3339), PurgeAt: purgeAt.Format(time.RFC3339)})
}

func (h handler) CancelDeletion(w http.ResponseWriter, r *http.Request) {
	ref, err := authenticatedMemberRef(h, r)
	if err != nil {
		presenter.Forbidden(w, "INVALID_TOKEN")
		return
	}
	if _, err := ref.Update(r.Context(), []firestore.Update{{Path: "deleted_at", Value: firestore.Delete}, {Path: "purge_at", Value: firestore.Delete}, {Path: "deletion_requested_at", Value: firestore.Delete}, {Path: "deletion_scheduled_at", Value: firestore.Delete}}); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.Success(w)
}

func (h handler) Restore(w http.ResponseWriter, r *http.Request) {
	ref, err := authenticatedMemberRef(h, r)
	if err != nil {
		presenter.Forbidden(w, "INVALID_TOKEN")
		return
	}
	snapshot, err := ref.Get(r.Context())
	if err != nil {
		presenter.Error(w, err)
		return
	}
	var member memberDoc
	if err := snapshot.DataTo(&member); err != nil {
		presenter.Error(w, err)
		return
	}
	if member.DeletedAt == nil {
		presenter.Success(w)
		return
	}
	if member.PurgeAt != nil && !time.Now().UTC().Before(*member.PurgeAt) {
		presenter.BadRequest(w, "ACCOUNT_RESTORE_EXPIRED")
		return
	}
	if _, err := ref.Update(r.Context(), []firestore.Update{
		{Path: "deleted_at", Value: firestore.Delete},
		{Path: "purge_at", Value: firestore.Delete},
		{Path: "deletion_requested_at", Value: firestore.Delete},
		{Path: "deletion_scheduled_at", Value: firestore.Delete},
	}); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.Success(w)
}
