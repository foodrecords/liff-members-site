package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"google.golang.org/api/iterator"
)

var rootCollections = []string{"members", "member_numbers", "reward_catalog", "serials", "square_pool", "kiosk_member_tokens", "kiosk_coupon_reservations"}

func main() {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		log.Fatal("FIRESTORE_EMULATOR_HOST is required; production migration is intentionally disabled")
	}
	config.FirebaseInit()
	defer config.FS.Close()
	ctx := context.Background()
	if _, err := config.OrganizationRef(config.FS).Set(ctx, map[string]interface{}{
		"organization_uuid":      config.OrganizationUUID(),
		"members_schema_version": 1,
	}, firestore.MergeAll); err != nil {
		log.Fatalf("initialize organization: %v", err)
	}
	for _, name := range rootCollections {
		count, err := copyCollection(ctx, config.FS.Collection(name), config.OrganizationCollection(config.FS, name))
		if err != nil {
			log.Fatalf("copy %s: %v", name, err)
		}
		fmt.Printf("%s: %d documents copied\n", name, count)
	}
	fmt.Printf("local data copied to organizations/%s (legacy documents retained)\n", config.OrganizationUUID())
}

func copyCollection(ctx context.Context, source, target *firestore.CollectionRef) (int, error) {
	iter := source.Documents(ctx)
	defer iter.Stop()
	count := 0
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			return count, nil
		}
		if err != nil {
			return count, err
		}
		targetDoc := target.Doc(doc.Ref.ID)
		if _, err := targetDoc.Set(ctx, doc.Data()); err != nil {
			return count, err
		}
		count++
		children, err := doc.Ref.Collections(ctx).GetAll()
		if err != nil {
			return count, err
		}
		for _, child := range children {
			childCount, err := copyCollection(ctx, child, targetDoc.Collection(child.ID))
			if err != nil {
				return count, err
			}
			count += childCount
		}
	}
}
