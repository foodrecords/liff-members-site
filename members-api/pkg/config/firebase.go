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

const DefaultOrganizationUUID = "35095fe0-1efc-40ff-bd13-9720c6d09e0f"

// OrganizationUUID is the contract and data-isolation boundary for the
// members service. Local development defaults to the existing FOOD RECORDS
// organization; production must set it explicitly before this layout is used.
func OrganizationUUID() string {
	if value := os.Getenv("ORGANIZATION_UUID"); value != "" {
		return value
	}
	return DefaultOrganizationUUID
}

func OrganizationRef(fs *firestore.Client) *firestore.DocumentRef {
	return fs.Collection("organizations").Doc(OrganizationUUID())
}

func OrganizationCollection(fs *firestore.Client, name string) *firestore.CollectionRef {
	return OrganizationRef(fs).Collection(name)
}

func OrganizationDataEnabled() bool {
	return os.Getenv("MEMBERS_DATA_LAYOUT") == "organization"
}

// DataCollection keeps production on the legacy layout until an explicit
// migration, while local development uses organization-scoped collections.
func DataCollection(fs *firestore.Client, name string) *firestore.CollectionRef {
	if OrganizationDataEnabled() {
		return OrganizationCollection(fs, name)
	}
	return fs.Collection(name)
}

func FirebaseInit() {
	conf := &firebase.Config{ProjectID: os.Getenv("PROJECT_ID")}

	var app *firebase.App
	var err error
	// dev
	if os.Getenv("ENV") == "local" && os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
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
