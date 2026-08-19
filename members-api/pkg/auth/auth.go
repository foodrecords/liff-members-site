package auth

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/presenter"
)

var (
	UID string
)

func JwtMiddleWare(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if err := verifyIDToken(ctx, idToken); err != nil {
			if regexp.MustCompile(`ID token has expired`).MatchString(err.Error()) {
				presenter.Forbidden(w, "token expired")
			} else {
				presenter.Forbidden(w, "invalid token")
			}
			return
		}
		next.ServeHTTP(w, r)
	}
}

func verifyIDToken(ctx context.Context, idToken string) error {

	client, err := config.FB.Auth(ctx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
		return err
	}

	token, err := client.VerifyIDToken(ctx, idToken)
	if err != nil {
		log.Printf("error verifying ID token: %v\n", err)
		return err
	}
	UID = token.UID

	return nil
}
