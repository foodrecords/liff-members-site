package coupon

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memberCouponDoc struct {
	Code           string    `firestore:"code"`
	Title          string    `firestore:"title"`
	Description    string    `firestore:"description"`
	DiscountAmount int       `firestore:"discount_amount"`
	DiscountLabel  string    `firestore:"discount_label"`
	ExpiresAt      time.Time `firestore:"expires_at"`
	PointMilestone int       `firestore:"point_milestone"`
	RewardID       string    `firestore:"reward_id"`
	PointCost      int       `firestore:"point_cost"`
	IssuedAt       time.Time `firestore:"issued_at"`
	Used           bool      `firestore:"used"`
	UsedAt         time.Time `firestore:"used_at"`
	ProductURL     string    `firestore:"product_url"`
	ImageURL       string    `firestore:"image_url"`
	BenefitType    string    `firestore:"benefit_type"`
	TargetType     string    `firestore:"target_type"`
	MaxUnitPrice   int       `firestore:"max_unit_price"`
	FreeQuantity   int       `firestore:"free_quantity"`
	Status         string    `firestore:"status"`
}

type CouponResp struct {
	ID             string `json:"id"`
	Code           string `json:"code,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	DiscountAmount int    `json:"discount_amount,omitempty"`
	DiscountLabel  string `json:"discount_label,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	PointMilestone int    `json:"point_milestone,omitempty"`
	RewardID       string `json:"reward_id,omitempty"`
	PointCost      int    `json:"point_cost,omitempty"`
	IssuedAt       string `json:"issued_at"`
	Used           bool   `json:"used"`
	UsedAt         string `json:"used_at,omitempty"`
	ProductURL     string `json:"product_url,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	BenefitType    string `json:"benefit_type,omitempty"`
	TargetType     string `json:"target_type,omitempty"`
	MaxUnitPrice   int    `json:"max_unit_price,omitempty"`
	FreeQuantity   int    `json:"free_quantity,omitempty"`
	Status         string `json:"status,omitempty"`
}

type GetResp struct {
	Coupons []CouponResp `json:"coupons"`
}

func (h handler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	prof, err := getProfile(idToken)
	if err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	memberRef := h.fs.Collection("members").Doc(prof.UserID)

	docs, err := memberRef.Collection("coupons").Documents(ctx).GetAll()
	if err != nil {
		if status.Code(err) != codes.NotFound {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
	}

	coupons := make([]CouponResp, 0, len(docs))
	for _, doc := range docs {
		var c memberCouponDoc
		if err := doc.DataTo(&c); err != nil {
			logger.Error(err.Error())
			continue
		}
		resp := CouponResp{
			ID:             doc.Ref.ID,
			Code:           c.Code,
			Title:          c.Title,
			Description:    c.Description,
			DiscountAmount: c.DiscountAmount,
			DiscountLabel:  c.DiscountLabel,
			PointMilestone: c.PointMilestone,
			RewardID:       c.RewardID,
			PointCost:      c.PointCost,
			IssuedAt:       c.IssuedAt.Format(time.RFC3339),
			Used:           c.Used,
			ProductURL:     c.ProductURL,
			ImageURL:       c.ImageURL,
			BenefitType:    c.BenefitType,
			TargetType:     c.TargetType,
			MaxUnitPrice:   c.MaxUnitPrice,
			FreeQuantity:   c.FreeQuantity,
			Status:         c.Status,
		}
		if !c.ExpiresAt.IsZero() {
			resp.ExpiresAt = c.ExpiresAt.Format(time.RFC3339)
		}
		if c.Used && !c.UsedAt.IsZero() {
			resp.UsedAt = c.UsedAt.Format(time.RFC3339)
		}
		coupons = append(coupons, resp)
	}

	presenter.EncodeWithMessage(w, GetResp{Coupons: coupons})
}

type getProfileResp struct {
	UserID        string `json:"userId"`
	DisplayName   string `json:"displayName"`
	PictureURL    string `json:"pictureUrl"`
	StatusMessage string `json:"statusMessage"`
}

func getProfile(token string) (*getProfileResp, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, _ := http.NewRequest("GET", "https://api.line.me/v2/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var profile getProfileResp
	if err := json.Unmarshal(b, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}
