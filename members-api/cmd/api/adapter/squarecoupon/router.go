package squarecoupon

import (
	"net/http"
	"os"
	"strings"

	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/square"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := newHandler()

	r.With(adminAuth).Post("/", h.Create)

	return r
}

type handler struct {
	sq *square.Loader
}

func newHandler() handler {
	return handler{sq: square.NewLoader()}
}

func adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || token != os.Getenv("ADMIN_TOKEN") {
			presenter.Forbidden(w, "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
