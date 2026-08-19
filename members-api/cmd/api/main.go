package main

import (
	"log"
	"net/http"
	"os"

	"github.com/foodrecords/members-api/cmd/api/adapter/coupon"
	"github.com/foodrecords/members-api/cmd/api/adapter/healthcheck"
	"github.com/foodrecords/members-api/cmd/api/adapter/kiosk"
	"github.com/foodrecords/members-api/cmd/api/adapter/member"
	"github.com/foodrecords/members-api/cmd/api/adapter/poolmonitor"
	"github.com/foodrecords/members-api/cmd/api/adapter/qrcode"
	"github.com/foodrecords/members-api/cmd/api/adapter/reward"
	"github.com/foodrecords/members-api/cmd/api/adapter/squarecoupon"
	"github.com/foodrecords/members-api/pkg/config"
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

	config.FirebaseInit()
	defer config.FS.Close()

	router := newRouter()

	code := server.Run(8080, router)
	os.Exit(code)
}

func newRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Members-Service-Key", "ngrok-skip-browser-warning"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Mount("/", healthcheck.NewRouter())
	r.Mount("/members", member.NewRouter())
	r.Mount("/qrcode", qrcode.NewRouter())
	r.Mount("/kiosk", kiosk.NewRouter())
	r.Mount("/coupons", coupon.NewRouter())
	r.Mount("/rewards", reward.NewRouter())
	r.Mount("/admin/square-coupons", squarecoupon.NewRouter())
	r.Mount("/admin/pool-monitor", poolmonitor.NewRouter())
	return r
}
