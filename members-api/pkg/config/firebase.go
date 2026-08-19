package config

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var FB *firebase.App
var FS *firestore.Client

func FirebaseInit() {
	conf := &firebase.Config{ProjectID: os.Getenv("PROJECT_ID")}

	var app *firebase.App
	var err error
	// dev
	if os.Getenv("ENV") == "local" {
		opt := option.WithCredentialsFile(os.Getenv("VOLUME_PATH") + "/key.json")
		app, err = firebase.NewApp(context.Background(), conf, opt)
	} else {
		app, err = firebase.NewApp(context.Background(), conf)
	}
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
	}
	FB = app

	client, err := app.Firestore(context.Background())
	if err != nil {
		log.Fatalf("error initializing firestore: %v\n", err)
	}
	FS = client
}
