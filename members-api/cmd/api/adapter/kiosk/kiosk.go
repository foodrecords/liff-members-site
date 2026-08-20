package kiosk

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/go-chi/chi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const tokenLifetime = 5 * time.Minute
const reservationLifetime = 10 * time.Minute

type memberTokenDoc struct {
	CheckoutID string    `firestore:"checkout_id"`
	UserID     string    `firestore:"user_id,omitempty"`
	ExpiresAt  time.Time `firestore:"expires_at"`
	LinkedAt   time.Time `firestore:"linked_at,omitempty"`
	ConsumedAt time.Time `firestore:"consumed_at,omitempty"`
}

type memberDoc struct {
	Number           int64  `firestore:"number"`
	Name             string `firestore:"name"`
	Point            int    `firestore:"point"`
	TotalEarnedPoint int    `firestore:"total_earned_point"`
}

type couponDoc struct {
	Title               string    `firestore:"title"`
	Description         string    `firestore:"description"`
	ExpiresAt           time.Time `firestore:"expires_at"`
	Used                bool      `firestore:"used"`
	UsedAt              time.Time `firestore:"used_at,omitempty"`
	Status              string    `firestore:"status"`
	BenefitType         string    `firestore:"benefit_type"`
	TargetType          string    `firestore:"target_type"`
	MaxUnitPrice        int       `firestore:"max_unit_price"`
	FreeQuantity        int       `firestore:"free_quantity"`
	DiscountAmount      int       `firestore:"discount_amount"`
	EligibleStoreIDs    []string  `firestore:"eligible_store_ids"`
	EligibleCategoryIDs []string  `firestore:"eligible_category_ids"`
	EligibleItemIDs     []string  `firestore:"eligible_item_ids"`
	EligibleOptionIDs   []string  `firestore:"eligible_option_ids"`
	ReservedBy          string    `firestore:"reserved_by_checkout_id,omitempty"`
	ReservedAt          time.Time `firestore:"reserved_at,omitempty"`
	ReservationExpires  time.Time `firestore:"reservation_expires_at,omitempty"`
	UsedOrderUUID       string    `firestore:"used_order_uuid,omitempty"`
}

type couponResponse struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	ExpiresAt           string   `json:"expires_at,omitempty"`
	BenefitType         string   `json:"benefit_type"`
	TargetType          string   `json:"target_type"`
	MaxUnitPrice        int      `json:"max_unit_price"`
	FreeQuantity        int      `json:"free_quantity"`
	EligibleStoreIDs    []string `json:"eligible_store_ids,omitempty"`
	EligibleCategoryIDs []string `json:"eligible_category_ids,omitempty"`
	EligibleItemIDs     []string `json:"eligible_item_ids,omitempty"`
	EligibleOptionIDs   []string `json:"eligible_option_ids,omitempty"`
}

type reservationDoc struct {
	MemberID   string    `firestore:"member_id"`
	CheckoutID string    `firestore:"checkout_id"`
	CouponIDs  []string  `firestore:"coupon_ids"`
	Status     string    `firestore:"status"`
	CreatedAt  time.Time `firestore:"created_at"`
	ExpiresAt  time.Time `firestore:"expires_at"`
	OrderUUID  string    `firestore:"order_uuid,omitempty"`
}

func (h handler) serviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv("MEMBERS_SERVICE_KEY")
		provided := r.Header.Get("X-Members-Service-Key")
		if expected == "" || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			presenter.Forbidden(w, "INVALID_SERVICE_KEY")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func lineProfile(token string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.line.me/v2/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("invalid LINE token")
	}
	var body struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.UserID == "" {
		return "", errors.New("missing LINE user")
	}
	return body.UserID, nil
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func (h handler) IssueCheckoutToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CheckoutID string `json:"checkout_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.CheckoutID == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}
	token, err := randomToken()
	if err != nil {
		presenter.Error(w, err)
		return
	}
	expires := time.Now().Add(tokenLifetime)
	if _, err := h.fs.Collection("kiosk_member_tokens").Doc(tokenHash(token)).Set(r.Context(), memberTokenDoc{CheckoutID: body.CheckoutID, ExpiresAt: expires}); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.EncodeWithMessage(w, map[string]interface{}{"token": token, "expires_at": expires.Format(time.RFC3339)})
}

func (h handler) LinkCheckout(w http.ResponseWriter, r *http.Request) {
	uid, err := lineProfile(bearer(r))
	if err != nil {
		presenter.Forbidden(w, "INVALID_TOKEN")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Token == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}
	doc := h.fs.Collection("kiosk_member_tokens").Doc(tokenHash(body.Token))
	err = h.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			return err
		}
		var link memberTokenDoc
		if err := snap.DataTo(&link); err != nil {
			return err
		}
		if link.ExpiresAt.Before(time.Now()) || !link.ConsumedAt.IsZero() {
			return errors.New("KIOSK_TOKEN_EXPIRED")
		}
		if link.UserID != "" && link.UserID != uid {
			return errors.New("KIOSK_ALREADY_LINKED")
		}
		link.UserID = uid
		if link.LinkedAt.IsZero() {
			link.LinkedAt = time.Now()
		}
		return tx.Set(doc, link)
	})
	if err != nil {
		presenter.BadRequest(w, err.Error())
		return
	}
	presenter.Success(w)
}

func (h handler) ResolveCheckout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token      string `json:"token"`
		CheckoutID string `json:"checkout_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" || body.CheckoutID == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}
	doc := h.fs.Collection("kiosk_member_tokens").Doc(tokenHash(body.Token))
	var token memberTokenDoc
	err := h.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			return err
		}
		if err := snap.DataTo(&token); err != nil {
			return err
		}
		if token.ExpiresAt.Before(time.Now()) || token.CheckoutID != body.CheckoutID {
			return errors.New("KIOSK_TOKEN_EXPIRED")
		}
		if token.UserID == "" {
			return nil
		}
		if token.ConsumedAt.IsZero() {
			token.ConsumedAt = time.Now()
		}
		return tx.Set(doc, token)
	})
	if err != nil {
		presenter.BadRequest(w, "KIOSK_TOKEN_EXPIRED")
		return
	}
	if token.UserID == "" {
		presenter.EncodeWithMessage(w, map[string]interface{}{"status": "pending"})
		return
	}
	memberRef := h.fs.Collection("members").Doc(token.UserID)
	memberSnap, err := memberRef.Get(r.Context())
	if err != nil {
		presenter.Error(w, err)
		return
	}
	var member memberDoc
	if err := memberSnap.DataTo(&member); err != nil {
		presenter.Error(w, err)
		return
	}
	docs, err := memberRef.Collection("coupons").Documents(r.Context()).GetAll()
	if err != nil {
		presenter.Error(w, err)
		return
	}
	now := time.Now()
	coupons := []couponResponse{}
	for _, snap := range docs {
		var c couponDoc
		if snap.DataTo(&c) != nil {
			continue
		}
		available := !c.Used && (c.Status == "" || c.Status == "available" || (c.Status == "reserved" && c.ReservationExpires.Before(now)))
		if !available || (!c.ExpiresAt.IsZero() && c.ExpiresAt.Before(now)) {
			continue
		}
		benefit := c.BenefitType
		if benefit == "" {
			benefit = "free_item_under_limit"
		}
		limit := c.MaxUnitPrice
		if limit == 0 {
			limit = c.DiscountAmount
		}
		qty := c.FreeQuantity
		if qty <= 0 {
			qty = 1
		}
		coupons = append(coupons, couponResponse{snap.Ref.ID, c.Title, c.Description, c.ExpiresAt.Format(time.RFC3339), benefit, defaultString(c.TargetType, "item_or_option"), limit, qty, c.EligibleStoreIDs, c.EligibleCategoryIDs, c.EligibleItemIDs, c.EligibleOptionIDs})
	}
	sort.Slice(coupons, func(i, j int) bool { return coupons[i].MaxUnitPrice > coupons[j].MaxUnitPrice })
	presenter.EncodeWithMessage(w, map[string]interface{}{"status": "linked", "member_session_id": tokenHash(body.Token), "member_number": member.Number, "point": member.Point, "coupons": coupons})
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (h handler) ReserveCoupons(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MemberSessionID string   `json:"member_session_id"`
		CheckoutID      string   `json:"checkout_id"`
		CouponIDs       []string `json:"coupon_ids"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.MemberSessionID == "" || body.CheckoutID == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}
	tokenSnap, err := h.fs.Collection("kiosk_member_tokens").Doc(body.MemberSessionID).Get(r.Context())
	if err != nil {
		presenter.BadRequest(w, "INVALID_MEMBER_SESSION")
		return
	}
	var token memberTokenDoc
	if tokenSnap.DataTo(&token) != nil || token.UserID == "" || token.ConsumedAt.IsZero() || token.CheckoutID != body.CheckoutID {
		presenter.BadRequest(w, "INVALID_MEMBER_SESSION")
		return
	}
	reservation := h.fs.Collection("kiosk_coupon_reservations").Doc(body.CheckoutID)
	selected := make([]couponResponse, 0, len(body.CouponIDs))
	err = h.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		now := time.Now()
		var previous reservationDoc
		previousSnap, previousErr := tx.Get(reservation)
		if previousErr != nil && status.Code(previousErr) != codes.NotFound {
			return previousErr
		}
		if previousErr == nil {
			if err := previousSnap.DataTo(&previous); err != nil {
				return err
			}
		}
		for _, id := range body.CouponIDs {
			ref := h.fs.Collection("members").Doc(token.UserID).Collection("coupons").Doc(id)
			snap, err := tx.Get(ref)
			if err != nil {
				return err
			}
			var c couponDoc
			if err := snap.DataTo(&c); err != nil {
				return err
			}
			if c.Used || (!c.ExpiresAt.IsZero() && c.ExpiresAt.Before(now)) || (c.Status == "reserved" && c.ReservedBy != body.CheckoutID && c.ReservationExpires.After(now)) {
				return errors.New("COUPON_UNAVAILABLE")
			}
			benefit := defaultString(c.BenefitType, "free_item_under_limit")
			limit := c.MaxUnitPrice
			if limit == 0 {
				limit = c.DiscountAmount
			}
			qty := c.FreeQuantity
			if qty <= 0 {
				qty = 1
			}
			selected = append(selected, couponResponse{ID: id, Title: c.Title, Description: c.Description, ExpiresAt: c.ExpiresAt.Format(time.RFC3339), BenefitType: benefit, TargetType: defaultString(c.TargetType, "item_or_option"), MaxUnitPrice: limit, FreeQuantity: qty, EligibleStoreIDs: c.EligibleStoreIDs, EligibleCategoryIDs: c.EligibleCategoryIDs, EligibleItemIDs: c.EligibleItemIDs, EligibleOptionIDs: c.EligibleOptionIDs})
		}
		selectedSet := map[string]bool{}
		for _, id := range body.CouponIDs {
			selectedSet[id] = true
		}
		for _, id := range previous.CouponIDs {
			if selectedSet[id] {
				continue
			}
			ref := h.fs.Collection("members").Doc(token.UserID).Collection("coupons").Doc(id)
			if err := tx.Update(ref, []firestore.Update{{Path: "status", Value: "available"}, {Path: "reserved_by_checkout_id", Value: ""}, {Path: "reservation_expires_at", Value: time.Time{}}}); err != nil {
				return err
			}
		}
		for _, id := range body.CouponIDs {
			ref := h.fs.Collection("members").Doc(token.UserID).Collection("coupons").Doc(id)
			if err := tx.Update(ref, []firestore.Update{{Path: "status", Value: "reserved"}, {Path: "reserved_by_checkout_id", Value: body.CheckoutID}, {Path: "reserved_at", Value: now}, {Path: "reservation_expires_at", Value: now.Add(reservationLifetime)}}); err != nil {
				return err
			}
		}
		return tx.Set(reservation, reservationDoc{MemberID: token.UserID, CheckoutID: body.CheckoutID, CouponIDs: body.CouponIDs, Status: "reserved", CreatedAt: now, ExpiresAt: now.Add(reservationLifetime)})
	})
	if err != nil {
		presenter.BadRequest(w, err.Error())
		return
	}
	presenter.EncodeWithMessage(w, map[string]interface{}{"reservation_id": body.CheckoutID, "expires_at": time.Now().Add(reservationLifetime).Format(time.RFC3339), "coupons": selected})
}

func (h handler) CancelReservation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.cancel(r.Context(), id); err != nil && status.Code(err) != codes.NotFound {
		presenter.Error(w, err)
		return
	}
	presenter.Success(w)
}

func (h handler) cancel(ctx context.Context, id string) error {
	ref := h.fs.Collection("kiosk_coupon_reservations").Doc(id)
	return h.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			return err
		}
		var res reservationDoc
		if err := snap.DataTo(&res); err != nil {
			return err
		}
		if res.Status != "reserved" {
			return nil
		}
		for _, cid := range res.CouponIDs {
			coupon := h.fs.Collection("members").Doc(res.MemberID).Collection("coupons").Doc(cid)
			if err := tx.Update(coupon, []firestore.Update{{Path: "status", Value: "available"}, {Path: "reserved_by_checkout_id", Value: ""}, {Path: "reservation_expires_at", Value: time.Time{}}}); err != nil {
				return err
			}
		}
		return tx.Update(ref, []firestore.Update{{Path: "status", Value: "canceled"}})
	})
}

func (h handler) FinalizeOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ReservationID string `json:"reservation_id"`
		OrderUUID     string `json:"order_uuid"`
		SquareOrderID string `json:"square_order_id"`
		LocationUUID  string `json:"location_uuid"`
		BaseAmount    int    `json:"base_amount"`
		Points        int    `json:"points"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.ReservationID == "" || body.OrderUUID == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}
	if body.Points <= 0 {
		body.Points = 100
	}
	resRef := h.fs.Collection("kiosk_coupon_reservations").Doc(body.ReservationID)
	err := h.fs.RunTransaction(r.Context(), func(ctx context.Context, tx *firestore.Transaction) error {
		resSnap, err := tx.Get(resRef)
		if err != nil {
			return err
		}
		var res reservationDoc
		if err := resSnap.DataTo(&res); err != nil {
			return err
		}
		if res.Status == "used" && res.OrderUUID == body.OrderUUID {
			return nil
		}
		if res.Status != "reserved" {
			return errors.New("RESERVATION_NOT_ACTIVE")
		}
		if res.ExpiresAt.Before(time.Now()) {
			return errors.New("RESERVATION_EXPIRED")
		}
		memberRef := h.fs.Collection("members").Doc(res.MemberID)
		memberSnap, err := tx.Get(memberRef)
		if err != nil {
			return err
		}
		var m memberDoc
		if err := memberSnap.DataTo(&m); err != nil {
			return err
		}
		pointLog := memberRef.Collection("point_logs").Doc("kiosk_" + body.OrderUUID)
		if _, err := tx.Get(pointLog); err != nil && status.Code(err) != codes.NotFound {
			return err
		} else if status.Code(err) == codes.NotFound {
			if err := tx.Update(memberRef, []firestore.Update{{Path: "point", Value: m.Point + body.Points}, {Path: "total_earned_point", Value: m.TotalEarnedPoint + body.Points}}); err != nil {
				return err
			}
			if err := tx.Create(pointLog, map[string]interface{}{"type": "purchase", "order_uuid": body.OrderUUID, "square_order_id": body.SquareOrderID, "location_uuid": body.LocationUUID, "base_amount": body.BaseAmount, "awarded_points": body.Points, "status": "granted", "created_at": time.Now()}); err != nil {
				return err
			}
		}
		for _, cid := range res.CouponIDs {
			coupon := memberRef.Collection("coupons").Doc(cid)
			if err := tx.Update(coupon, []firestore.Update{{Path: "status", Value: "used"}, {Path: "used", Value: true}, {Path: "used_at", Value: time.Now()}, {Path: "used_order_uuid", Value: body.OrderUUID}}); err != nil {
				return err
			}
		}
		return tx.Update(resRef, []firestore.Update{{Path: "status", Value: "used"}, {Path: "order_uuid", Value: body.OrderUUID}})
	})
	if err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.EncodeWithMessage(w, map[string]interface{}{"awarded_points": body.Points})
}
