// Command migrate-production inventories, copies, and verifies the members
// Firestore data when moving from fr-agaruke to an organization-scoped layout.
// It never writes unless --mode=copy and --apply are both provided.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

const (
	defaultSourceProject = "fr-agaruke"
	defaultTargetProject = "food-records-prod"
	defaultOrganization  = "35095fe0-1efc-40ff-bd13-9720c6d09e0f"
)

var durableRoots = []string{"members", "member_numbers", "reward_catalog", "serials", "square_pool"}
var transientRoots = []string{"kiosk_member_tokens", "kiosk_coupon_reservations"}

type options struct {
	mode, sourceProject, targetProject, organization, reportPath string
	apply, overwriteConflicts                                    bool
}

type record struct {
	Path string
	Data map[string]interface{}
	Hash string
}

type collectionSummary struct {
	Documents int `json:"documents"`
}

type integritySummary struct {
	Members                       int   `json:"members"`
	MemberNumbers                 int   `json:"member_numbers"`
	MemberNumberMissing           int   `json:"member_number_missing"`
	MemberNumberMismatch          int   `json:"member_number_mismatch"`
	MemberNumberOrphan            int   `json:"member_number_orphan"`
	MemberNumberDuplicate         int   `json:"member_number_duplicate"`
	PointBalanceTotal             int64 `json:"point_balance_total"`
	TotalEarnedPointTotal         int64 `json:"total_earned_point_total"`
	Coupons                       int   `json:"coupons"`
	CouponsUsed                   int   `json:"coupons_used"`
	CouponRewardMissing           int   `json:"coupon_reward_missing"`
	PointLogs                     int   `json:"point_logs"`
	SerialUserScans               int   `json:"serial_user_scans"`
	DocumentReferences            int   `json:"document_references"`
	ActiveKioskMemberTokens       int   `json:"active_kiosk_member_tokens"`
	ActiveKioskCouponReservations int   `json:"active_kiosk_coupon_reservations"`
	SquarePoolUsed                int   `json:"square_pool_used"`
	SerialsUsed                   int   `json:"serials_used"`
}

type sideReport struct {
	Project     string                       `json:"project"`
	BasePath    string                       `json:"base_path"`
	Collections map[string]collectionSummary `json:"collections"`
	Integrity   integritySummary             `json:"integrity"`
	Digest      string                       `json:"digest"`
}

type comparison struct {
	SourceDocuments int      `json:"source_documents"`
	TargetDocuments int      `json:"target_documents"`
	MissingInTarget int      `json:"missing_in_target"`
	ExtraInTarget   int      `json:"extra_in_target"`
	ContentMismatch int      `json:"content_mismatch"`
	MismatchIDs     []string `json:"mismatch_path_hashes,omitempty"`
}

type migrationReport struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Mode        string     `json:"mode"`
	Source      sideReport `json:"source"`
	Target      sideReport `json:"target"`
	Comparison  comparison `json:"comparison"`
	Copied      int        `json:"copied,omitempty"`
	Skipped     int        `json:"skipped,omitempty"`
	Notes       []string   `json:"notes"`
}

func main() {
	opts := parseFlags()
	if err := run(context.Background(), opts); err != nil {
		log.Fatal(err)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.mode, "mode", "inventory", "inventory, copy, or verify")
	flag.StringVar(&opts.sourceProject, "source-project", defaultSourceProject, "legacy Firestore project")
	flag.StringVar(&opts.targetProject, "target-project", defaultTargetProject, "destination Firestore project")
	flag.StringVar(&opts.organization, "organization", defaultOrganization, "destination organization UUID")
	flag.StringVar(&opts.reportPath, "report", "-", "JSON report path, or - for stdout")
	flag.BoolVar(&opts.apply, "apply", false, "allow writes in copy mode")
	flag.BoolVar(&opts.overwriteConflicts, "overwrite-conflicts", false, "overwrite destination documents whose content differs")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) error {
	if opts.mode != "inventory" && opts.mode != "copy" && opts.mode != "verify" {
		return fmt.Errorf("invalid mode %q", opts.mode)
	}
	if opts.sourceProject == opts.targetProject {
		return errors.New("source and target projects must differ")
	}
	if strings.TrimSpace(opts.organization) == "" {
		return errors.New("organization is required")
	}
	if opts.mode == "copy" {
		if !opts.apply {
			return errors.New("copy mode requires --apply")
		}
		want := opts.sourceProject + "->" + opts.targetProject + "/" + opts.organization
		if os.Getenv("MEMBERS_MIGRATION_CONFIRM") != want {
			return fmt.Errorf("copy mode requires MEMBERS_MIGRATION_CONFIRM=%q", want)
		}
	}

	source, err := firestore.NewClient(ctx, opts.sourceProject)
	if err != nil {
		return fmt.Errorf("open source Firestore: %w", err)
	}
	defer source.Close()
	target, err := firestore.NewClient(ctx, opts.targetProject)
	if err != nil {
		return fmt.Errorf("open target Firestore: %w", err)
	}
	defer target.Close()

	sourceRecords, err := readSide(ctx, source, opts.sourceProject, nil, append(append([]string{}, durableRoots...), transientRoots...))
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	targetBase := target.Collection("organizations").Doc(opts.organization)
	targetRecords, err := readSide(ctx, target, opts.targetProject, targetBase, append(append([]string{}, durableRoots...), transientRoots...))
	if err != nil {
		return fmt.Errorf("read target: %w", err)
	}

	report := migrationReport{
		GeneratedAt: time.Now().UTC(),
		Mode:        opts.mode,
		Source:      buildSideReport(opts.sourceProject, "/", sourceRecords),
		Target:      buildSideReport(opts.targetProject, "organizations/"+opts.organization, targetRecords),
		Comparison:  compare(filterDurable(sourceRecords), filterDurable(targetRecords)),
		Notes: []string{
			"Reports contain aggregate counts and hashed mismatch paths; document data and IDs are not emitted.",
			"kiosk_member_tokens and kiosk_coupon_reservations are inventoried but intentionally not copied; drain them before cutover.",
		},
	}

	if opts.mode == "copy" {
		if report.Source.Integrity.DocumentReferences != 0 {
			return errors.New("source contains Firestore document references; migration is blocked until their destination mapping is reviewed")
		}
		if report.Comparison.ExtraInTarget != 0 {
			return errors.New("destination contains documents absent from the source; copy is blocked until they are reviewed")
		}
		copied, skipped, err := copyRecords(ctx, target, targetBase, filterDurable(sourceRecords), filterDurable(targetRecords), opts.overwriteConflicts)
		if err != nil {
			return err
		}
		report.Copied, report.Skipped = copied, skipped
		allRoots := append(append([]string{}, durableRoots...), transientRoots...)
		verifiedSource, err := readSide(ctx, source, opts.sourceProject, nil, allRoots)
		if err != nil {
			return fmt.Errorf("reread source after copy: %w", err)
		}
		verifiedTarget, err := readSide(ctx, target, opts.targetProject, targetBase, allRoots)
		if err != nil {
			return fmt.Errorf("read destination after copy: %w", err)
		}
		report.Source = buildSideReport(opts.sourceProject, "/", verifiedSource)
		report.Target = buildSideReport(opts.targetProject, "organizations/"+opts.organization, verifiedTarget)
		report.Comparison = compare(filterDurable(verifiedSource), filterDurable(verifiedTarget))
	}

	if err := writeReport(opts.reportPath, report); err != nil {
		return err
	}
	if opts.mode == "verify" || opts.mode == "copy" {
		if report.Comparison.MissingInTarget != 0 || report.Comparison.ExtraInTarget != 0 || report.Comparison.ContentMismatch != 0 {
			return errors.New("source and destination verification failed; see report")
		}
	}
	return nil
}

func readSide(ctx context.Context, client *firestore.Client, project string, base *firestore.DocumentRef, roots []string) (map[string]record, error) {
	result := make(map[string]record)
	for _, root := range roots {
		log.Printf("inventory %s: %s", project, root)
		var collection *firestore.CollectionRef
		if base == nil {
			collection = client.Collection(root)
		} else {
			collection = base.Collection(root)
		}
		if err := readCollection(ctx, collection, root, result); err != nil {
			return nil, err
		}
		log.Printf("inventory %s: %s complete", project, root)
	}
	for _, child := range []struct {
		name string
		root string
	}{{"coupons", "members"}, {"point_logs", "members"}, {"user_scans", "serials"}} {
		if !contains(roots, child.root) {
			continue
		}
		log.Printf("inventory %s: collection group %s", project, child.name)
		if err := readCollectionGroup(ctx, client, base, child.root, child.name, result); err != nil {
			return nil, err
		}
		log.Printf("inventory %s: collection group %s complete", project, child.name)
	}
	return result, nil
}

func readCollection(ctx context.Context, collection *firestore.CollectionRef, logicalCollectionPath string, result map[string]record) error {
	iter := collection.Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
		path := logicalCollectionPath + "/" + doc.Ref.ID
		data := doc.Data()
		result[path] = record{Path: path, Data: data, Hash: dataHash(data)}
	}
}

func readCollectionGroup(ctx context.Context, client *firestore.Client, base *firestore.DocumentRef, root, group string, result map[string]record) error {
	iter := client.CollectionGroup(group).Documents(ctx)
	defer iter.Stop()
	prefix := root + "/"
	if base != nil {
		prefix = base.Path + "/" + prefix
	}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
		if !strings.HasPrefix(doc.Ref.Path, prefix) {
			continue
		}
		logicalPath := doc.Ref.Path
		if base != nil {
			logicalPath = strings.TrimPrefix(logicalPath, base.Path+"/")
		}
		data := doc.Data()
		result[logicalPath] = record{Path: logicalPath, Data: data, Hash: dataHash(data)}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func filterDurable(records map[string]record) map[string]record {
	result := make(map[string]record)
	for path, item := range records {
		root := strings.SplitN(path, "/", 2)[0]
		for _, durable := range durableRoots {
			if root == durable {
				result[path] = item
				break
			}
		}
	}
	return result
}

func copyRecords(ctx context.Context, targetClient *firestore.Client, targetBase *firestore.DocumentRef, source, target map[string]record, overwrite bool) (int, int, error) {
	paths := sortedPaths(source)
	for _, path := range paths {
		if existing, ok := target[path]; ok && existing.Hash != source[path].Hash && !overwrite {
			return 0, 0, fmt.Errorf("destination conflict at hashed path %s; rerun only after review, or use --overwrite-conflicts", pathHash(path))
		}
	}
	batch := targetClient.Batch()
	pending, copied, skipped := 0, 0, 0
	commit := func() error {
		if pending == 0 {
			return nil
		}
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
		batch = targetClient.Batch()
		pending = 0
		return nil
	}
	for _, path := range paths {
		item := source[path]
		if existing, ok := target[path]; ok && existing.Hash == item.Hash {
			skipped++
			continue
		}
		ref, err := logicalDocumentRef(targetBase, path)
		if err != nil {
			return copied, skipped, err
		}
		batch.Set(ref, item.Data)
		pending++
		copied++
		if pending >= 400 {
			if err := commit(); err != nil {
				return copied, skipped, fmt.Errorf("commit copy batch: %w", err)
			}
		}
	}
	if err := commit(); err != nil {
		return copied, skipped, fmt.Errorf("commit final copy batch: %w", err)
	}
	return copied, skipped, nil
}

func logicalDocumentRef(base *firestore.DocumentRef, path string) (*firestore.DocumentRef, error) {
	parts := strings.Split(path, "/")
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("invalid logical document path %q", path)
	}
	ref := base.Collection(parts[0]).Doc(parts[1])
	for index := 2; index < len(parts); index += 2 {
		ref = ref.Collection(parts[index]).Doc(parts[index+1])
	}
	return ref, nil
}

func buildSideReport(project, base string, records map[string]record) sideReport {
	report := sideReport{Project: project, BasePath: base, Collections: make(map[string]collectionSummary)}
	memberNumbers := make(map[string]string)
	numberCountByUser := make(map[string]int)
	members := make(map[string]string)
	rewards := make(map[string]bool)
	for path, item := range records {
		parts := strings.Split(path, "/")
		collection := parts[len(parts)-2]
		summary := report.Collections[collection]
		summary.Documents++
		report.Collections[collection] = summary
		report.Integrity.DocumentReferences += countDocumentRefs(item.Data)
		if len(parts) == 2 && parts[0] == "members" {
			report.Integrity.Members++
			number := paddedNumber(item.Data["number"])
			members[parts[1]] = number
			report.Integrity.PointBalanceTotal += integer(item.Data["point"])
			total := integer(item.Data["total_earned_point"])
			if total == 0 && integer(item.Data["point"]) > 0 {
				total = integer(item.Data["point"])
			}
			report.Integrity.TotalEarnedPointTotal += total
		}
		if len(parts) == 2 && parts[0] == "member_numbers" {
			report.Integrity.MemberNumbers++
			userID := stringValue(item.Data["user_id"])
			memberNumbers[parts[0]+"/"+parts[1]] = userID
			numberCountByUser[userID]++
		}
		if len(parts) == 2 && parts[0] == "reward_catalog" {
			rewards[parts[1]] = true
		}
		if collection == "coupons" {
			report.Integrity.Coupons++
			if boolean(item.Data["used"]) {
				report.Integrity.CouponsUsed++
			}
			rewardID := stringValue(item.Data["reward_id"])
			if rewardID != "" && !rewards[rewardID] {
				// Rechecked below after all reward documents have been visited.
			}
		}
		if collection == "point_logs" {
			report.Integrity.PointLogs++
		}
		if collection == "user_scans" {
			report.Integrity.SerialUserScans++
		}
		if len(parts) == 2 && parts[0] == "kiosk_member_tokens" && futureTimestamp(item.Data["expires_at"]) {
			report.Integrity.ActiveKioskMemberTokens++
		}
		if len(parts) == 2 && parts[0] == "kiosk_coupon_reservations" && futureTimestamp(item.Data["expires_at"]) {
			report.Integrity.ActiveKioskCouponReservations++
		}
		if len(parts) == 2 && parts[0] == "square_pool" && boolean(item.Data["used"]) {
			report.Integrity.SquarePoolUsed++
		}
		if len(parts) == 2 && parts[0] == "serials" && boolean(item.Data["used"]) {
			report.Integrity.SerialsUsed++
		}
	}
	for userID, number := range members {
		if number == "" {
			report.Integrity.MemberNumberMissing++
			continue
		}
		indexedUser, ok := memberNumbers["member_numbers/"+number]
		if !ok {
			report.Integrity.MemberNumberMissing++
		} else if indexedUser != userID {
			report.Integrity.MemberNumberMismatch++
		}
	}
	for _, userID := range memberNumbers {
		if _, ok := members[userID]; !ok {
			report.Integrity.MemberNumberOrphan++
		}
	}
	for _, count := range numberCountByUser {
		if count > 1 {
			report.Integrity.MemberNumberDuplicate += count - 1
		}
	}
	for path, item := range records {
		parts := strings.Split(path, "/")
		if parts[len(parts)-2] != "coupons" {
			continue
		}
		rewardID := stringValue(item.Data["reward_id"])
		if rewardID != "" && !rewards[rewardID] {
			report.Integrity.CouponRewardMissing++
		}
	}
	h := sha256.New()
	for _, path := range sortedPaths(records) {
		_, _ = h.Write([]byte(path + "\x00" + records[path].Hash + "\n"))
	}
	report.Digest = hex.EncodeToString(h.Sum(nil))
	return report
}

func compare(source, target map[string]record) comparison {
	result := comparison{SourceDocuments: len(source), TargetDocuments: len(target)}
	for path, src := range source {
		dst, ok := target[path]
		if !ok {
			result.MissingInTarget++
			result.MismatchIDs = append(result.MismatchIDs, pathHash(path))
		} else if src.Hash != dst.Hash {
			result.ContentMismatch++
			result.MismatchIDs = append(result.MismatchIDs, pathHash(path))
		}
	}
	for path := range target {
		if _, ok := source[path]; !ok {
			result.ExtraInTarget++
			result.MismatchIDs = append(result.MismatchIDs, pathHash(path))
		}
	}
	sort.Strings(result.MismatchIDs)
	if len(result.MismatchIDs) > 100 {
		result.MismatchIDs = result.MismatchIDs[:100]
	}
	return result
}

func dataHash(data map[string]interface{}) string {
	normalized := normalize(reflect.ValueOf(data))
	raw, err := json.Marshal(normalized)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalize(value reflect.Value) interface{} {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		if ref, ok := value.Interface().(*firestore.DocumentRef); ok {
			return map[string]string{"__document_ref__": ref.Path}
		}
		return normalize(value.Elem())
	}
	if value.CanInterface() {
		if timestamp, ok := value.Interface().(time.Time); ok {
			return map[string]string{"__timestamp__": timestamp.UTC().Format(time.RFC3339Nano)}
		}
		if bytes, ok := value.Interface().([]byte); ok {
			return map[string]string{"__bytes__": hex.EncodeToString(bytes)}
		}
	}
	switch value.Kind() {
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
		result := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			result[fmt.Sprint(key.Interface())] = normalize(value.MapIndex(key))
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]interface{}, value.Len())
		for index := range result {
			result[index] = normalize(value.Index(index))
		}
		return result
	case reflect.Struct:
		result := make(map[string]interface{})
		typeInfo := value.Type()
		for index := 0; index < value.NumField(); index++ {
			if typeInfo.Field(index).PkgPath == "" {
				result[typeInfo.Field(index).Name] = normalize(value.Field(index))
			}
		}
		return result
	default:
		if value.CanInterface() {
			return value.Interface()
		}
		return fmt.Sprint(value)
	}
}

func countDocumentRefs(value interface{}) int {
	if _, ok := value.(*firestore.DocumentRef); ok {
		return 1
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return 0
	}
	if rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return 0
		}
		return countDocumentRefs(rv.Elem().Interface())
	}
	total := 0
	switch rv.Kind() {
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			total += countDocumentRefs(rv.MapIndex(key).Interface())
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < rv.Len(); index++ {
			total += countDocumentRefs(rv.Index(index).Interface())
		}
	}
	return total
}

func sortedPaths(records map[string]record) []string {
	paths := make([]string, 0, len(records))
	for path := range records {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		leftDepth, rightDepth := strings.Count(paths[i], "/"), strings.Count(paths[j], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return paths[i] < paths[j]
	})
	return paths
}

func pathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

func integer(value interface{}) int64 {
	switch number := value.(type) {
	case int:
		return int64(number)
	case int32:
		return int64(number)
	case int64:
		return number
	case float64:
		return int64(number)
	default:
		return 0
	}
}

func paddedNumber(value interface{}) string {
	if value == nil {
		return ""
	}
	switch value.(type) {
	case int, int32, int64, float64:
	default:
		return ""
	}
	number := integer(value)
	if number < 0 || number > 999999 {
		return ""
	}
	return fmt.Sprintf("%06d", number)
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolean(value interface{}) bool {
	result, _ := strconv.ParseBool(fmt.Sprint(value))
	return result
}

func futureTimestamp(value interface{}) bool {
	timestamp, ok := value.(time.Time)
	return ok && timestamp.After(time.Now())
}

func writeReport(path string, report migrationReport) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
