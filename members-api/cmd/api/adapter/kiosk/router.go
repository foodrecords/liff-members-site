package kiosk

import (
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := handler{fs: config.FS}
	r.Post("/checkouts/link", h.LinkCheckout)
	r.Group(func(internal chi.Router) {
		internal.Use(h.serviceAuth)
		internal.Post("/members/resolve-line", h.ResolveLineMember)
		internal.Post("/checkout-tokens", h.IssueCheckoutToken)
		internal.Post("/checkouts/resolve", h.ResolveCheckout)
		internal.Post("/coupon-reservations", h.ReserveCoupons)
		internal.Post("/coupon-reservations/{id}/cancel", h.CancelReservation)
		internal.Post("/orders/finalize", h.FinalizeOrder)
	})
	return r
}

type handler struct{ fs *firestore.Client }
