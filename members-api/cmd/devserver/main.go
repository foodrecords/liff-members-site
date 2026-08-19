package main

import (
	"log"
	"net/http"
	"os"

	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/server"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
)

func main() {
	flush, err := logger.Setup()
	if err != nil {
		log.Fatal(err)
	}
	defer flush()

	dbPath := "testdata/db.json"
	if p := os.Getenv("DEV_DB_PATH"); p != "" {
		dbPath = p
	}

	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("store init: %v", err)
	}

	router := newRouter(store)

	log.Printf("dev server: http://localhost:8081")
	log.Printf("dev tools:  http://localhost:8081/dev/serials")
	code := server.Run(8081, router)
	os.Exit(code)
}

func newRouter(store *Store) http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))

	h := &handler{store: store}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/members", h.getMember)
	r.Post("/qrcode", h.postQRCode)
	r.Get("/coupons", h.getCoupons)
	r.Post("/coupons/{id}/use", h.useCoupon)
	r.Get("/rewards", h.getRewards)
	r.Post("/rewards/{id}/exchange", h.exchangeReward)

	r.Get("/dev/users", h.devListUsers)
	r.Get("/dev/serials", h.devListSerials)
	r.Post("/dev/serials", h.devCreateSerial)
	r.Post("/dev/reset", h.devReset)

	return r
}
