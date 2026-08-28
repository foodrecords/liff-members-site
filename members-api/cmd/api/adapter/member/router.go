package member

import (
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/square"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := newHandler()

	r.Get("/", h.Get)
	r.Patch("/", h.Patch)
	r.Delete("/", h.ScheduleDeletion)
	r.Post("/deletion/cancel", h.CancelDeletion)
	r.Post("/restore", h.Restore)
	r.Post("/register", h.Register)

	return r
}

type handler struct {
	fs   *firestore.Client
	pool *square.Pool
}

func newHandler() handler {
	return handler{
		fs:   config.FS,
		pool: square.NewPool(config.FS),
	}
}
