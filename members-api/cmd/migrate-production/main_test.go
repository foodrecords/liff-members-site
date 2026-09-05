package main

import "testing"

func TestRelativeFirestorePath(t *testing.T) {
	tests := map[string]string{
		"members/user/coupons/coupon": "members/user/coupons/coupon",
		"projects/source/databases/(default)/documents/members/user/coupons/coupon": "members/user/coupons/coupon",
		"projects/target/databases/(default)/documents/organizations/org":           "organizations/org",
		"/members/user/point_logs/log":                                              "members/user/point_logs/log",
	}
	for input, want := range tests {
		if got := relativeFirestorePath(input); got != want {
			t.Fatalf("relativeFirestorePath(%q) = %q, want %q", input, got, want)
		}
	}
}
