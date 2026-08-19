package coupon

import (
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := newHandler()

	r.Get("/", h.Get)
	r.Post("/{id}/use", h.Use)

	return r
}

type handler struct {
	fs *firestore.Client
}

func newHandler() handler {
	return handler{
		fs: config.FS,
	}
}
