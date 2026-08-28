package main

import (
	"testing"
	"time"
)

func TestDataHashIgnoresMapOrder(t *testing.T) {
	left := map[string]interface{}{"name": "member", "point": int64(10), "at": time.Unix(100, 20)}
	right := map[string]interface{}{"at": time.Unix(100, 20), "point": int64(10), "name": "member"}
	if dataHash(left) != dataHash(right) {
		t.Fatal("equivalent Firestore data must have the same hash")
	}
}

func TestCompareDoesNotExposeDocumentPaths(t *testing.T) {
	source := map[string]record{"members/LINE_USER_ID": {Path: "members/LINE_USER_ID", Hash: "source"}}
	target := map[string]record{"members/LINE_USER_ID": {Path: "members/LINE_USER_ID", Hash: "target"}}
	result := compare(source, target)
	if result.ContentMismatch != 1 || len(result.MismatchIDs) != 1 {
		t.Fatalf("unexpected comparison: %+v", result)
	}
	if result.MismatchIDs[0] == "members/LINE_USER_ID" {
		t.Fatal("report must not expose document path")
	}
}

func TestLogicalDocumentRefRejectsCollectionPath(t *testing.T) {
	if _, err := logicalDocumentRef(nil, "members"); err == nil {
		t.Fatal("collection path must be rejected")
	}
}

func TestBuildSideReportChecksMemberNumberIntegrity(t *testing.T) {
	records := map[string]record{
		"members/U1":            {Path: "members/U1", Data: map[string]interface{}{"number": int64(42), "point": int64(5), "total_earned_point": int64(8)}},
		"member_numbers/000042": {Path: "member_numbers/000042", Data: map[string]interface{}{"user_id": "U1"}},
	}
	report := buildSideReport("test", "/", records)
	if report.Integrity.MemberNumberMissing != 0 || report.Integrity.MemberNumberMismatch != 0 || report.Integrity.MemberNumberOrphan != 0 {
		t.Fatalf("unexpected integrity report: %+v", report.Integrity)
	}
	if report.Integrity.PointBalanceTotal != 5 || report.Integrity.TotalEarnedPointTotal != 8 {
		t.Fatalf("unexpected point totals: %+v", report.Integrity)
	}
}

func TestBuildSideReportDetectsDuplicateMemberNumbers(t *testing.T) {
	records := map[string]record{
		"members/U1":            {Path: "members/U1", Data: map[string]interface{}{"number": int64(42)}},
		"member_numbers/000042": {Path: "member_numbers/000042", Data: map[string]interface{}{"user_id": "U1"}},
		"member_numbers/000099": {Path: "member_numbers/000099", Data: map[string]interface{}{"user_id": "U1"}},
	}
	report := buildSideReport("test", "/", records)
	if report.Integrity.MemberNumberDuplicate != 1 {
		t.Fatalf("expected one duplicate index, got %+v", report.Integrity)
	}
}
