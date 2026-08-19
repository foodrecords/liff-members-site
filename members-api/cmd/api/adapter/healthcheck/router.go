package healthcheck

import (
	"net/http"

	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := newHandler()

	r.Get("/", h.Get)

	return r
}

type handler struct {
}

func newHandler() handler {
	return handler{}
}

func (h handler) Get(w http.ResponseWriter, _ *http.Request) {
	presenter.Success(w)
}
