package member

import (
	"encoding/json"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
)

type PatchReq struct {
	Name string `json:"name"`
}

func (h handler) Patch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	prof, err := getProfile(idToken)
	if err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	var req PatchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		presenter.BadRequest(w, "INVALID_REQUEST")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		presenter.BadRequest(w, "NAME_REQUIRED")
		return
	}
	if len([]rune(name)) > 20 {
		presenter.BadRequest(w, "NAME_TOO_LONG")
		return
	}

	memberRef := h.fs.Collection("members").Doc(prof.UserID)
	if _, err := memberRef.Update(ctx, []firestore.Update{
		{Path: "name", Value: name},
	}); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	presenter.Success(w)
}
