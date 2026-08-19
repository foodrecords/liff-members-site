package poolmonitor

import (
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/go-chi/chi"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	h := newHandler()

	r.With(adminAuth).Get("/", h.Status)

	return r
}

type handler struct {
	fs *firestore.Client
}

func newHandler() handler {
	return handler{fs: config.FS}
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
