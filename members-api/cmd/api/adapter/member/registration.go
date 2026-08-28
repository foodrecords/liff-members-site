package member

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/presenter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CurrentTermsVersion   = "2026-08-28"
	CurrentPrivacyVersion = "2026-08-28"
	MobileOrderSource     = "mobile_order_liff"
)

type registrationRequest struct {
	TermsVersion   string `json:"terms_version"`
	PrivacyVersion string `json:"privacy_policy_version"`
	ConsentSource  string `json:"consent_source"`
}

type registrationConsent struct {
	TermsVersion   string
	PrivacyVersion string
	Source         string
	ConsentedAt    time.Time
}

type registrationConsentKey struct{}

func registrationConsentFromContext(ctx context.Context) (registrationConsent, bool) {
	value, ok := ctx.Value(registrationConsentKey{}).(registrationConsent)
	return value, ok
}

func (h handler) Register(w http.ResponseWriter, r *http.Request) {
	var body registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.TermsVersion != CurrentTermsVersion ||
		body.PrivacyVersion != CurrentPrivacyVersion ||
		body.ConsentSource != MobileOrderSource {
		presenter.BadRequest(w, "MEMBER_CONSENT_REQUIRED")
		return
	}
	ref, err := authenticatedMemberRef(h, r)
	if err != nil {
		presenter.Forbidden(w, "INVALID_TOKEN")
		return
	}
	now := time.Now().UTC()
	updates := []firestore.Update{
		{Path: "terms_version", Value: body.TermsVersion},
		{Path: "privacy_policy_version", Value: body.PrivacyVersion},
		{Path: "consented_at", Value: now},
		{Path: "consent_source", Value: body.ConsentSource},
	}
	snapshot, getErr := ref.Get(r.Context())
	if getErr == nil {
		var member memberDoc
		if err := snapshot.DataTo(&member); err != nil {
			presenter.Error(w, err)
			return
		}
		if member.DeletedAt != nil {
			if member.PurgeAt != nil && !now.Before(*member.PurgeAt) {
				presenter.BadRequest(w, "ACCOUNT_RESTORE_EXPIRED")
				return
			}
			updates = append(updates,
				firestore.Update{Path: "deleted_at", Value: firestore.Delete},
				firestore.Update{Path: "purge_at", Value: firestore.Delete},
			)
		}
		if _, err := ref.Update(r.Context(), updates); err != nil {
			presenter.Error(w, err)
			return
		}
		h.Get(w, r)
		return
	}
	if status.Code(getErr) != codes.NotFound {
		presenter.Error(w, getErr)
		return
	}
	consent := registrationConsent{TermsVersion: body.TermsVersion, PrivacyVersion: body.PrivacyVersion, Source: body.ConsentSource, ConsentedAt: now}
	h.Get(w, r.WithContext(context.WithValue(r.Context(), registrationConsentKey{}, consent)))
}
