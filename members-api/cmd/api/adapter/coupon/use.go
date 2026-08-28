package coupon

import (
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/go-chi/chi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h handler) Use(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	prof, err := getProfile(idToken)
	if err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	couponRef := config.DataCollection(h.fs, "members").Doc(prof.UserID).Collection("coupons").Doc(id)
	snap, err := couponRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			presenter.BadRequest(w, "クーポンが見つかりません")
			return
		}
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var c memberCouponDoc
	if err := snap.DataTo(&c); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	if c.Used {
		presenter.BadRequest(w, "このクーポンは既に使用済みです")
		return
	}

	_, err = couponRef.Update(ctx, []firestore.Update{
		{Path: "used", Value: true},
		{Path: "used_at", Value: time.Now()},
	})
	if err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	presenter.Success(w)
}
