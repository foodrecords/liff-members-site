package reward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/square"
	"github.com/go-chi/chi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errInsufficientPoints = errors.New("insufficient points")

type memberDocExchange struct {
	Point int `firestore:"point"`
}

type memberCouponDoc struct {
	Title               string    `firestore:"title"`
	Description         string    `firestore:"description"`
	ImageURL            string    `firestore:"image_url"`
	RewardID            string    `firestore:"reward_id"`
	PointCost           int       `firestore:"point_cost"`
	IssuedAt            time.Time `firestore:"issued_at"`
	ExpiresAt           time.Time `firestore:"expires_at"`
	Used                bool      `firestore:"used"`
	SquareDiscountCode  string    `firestore:"square_discount_code,omitempty"`
	ProductURL          string    `firestore:"product_url,omitempty"`
	BenefitType         string    `firestore:"benefit_type"`
	TargetType          string    `firestore:"target_type"`
	MaxUnitPrice        int       `firestore:"max_unit_price"`
	FreeQuantity        int       `firestore:"free_quantity"`
	EligibleStoreIDs    []string  `firestore:"eligible_store_ids"`
	EligibleCategoryIDs []string  `firestore:"eligible_category_ids"`
	EligibleItemIDs     []string  `firestore:"eligible_item_ids"`
	EligibleOptionIDs   []string  `firestore:"eligible_option_ids"`
	Status              string    `firestore:"status"`
}

type ExchangeResp struct {
	CouponID           string `json:"coupon_id"`
	Title              string `json:"title"`
	PointCost          int    `json:"point_cost"`
	NewPoint           int    `json:"new_point"`
	SquareDiscountCode string `json:"square_discount_code,omitempty"`
	ProductURL         string `json:"product_url,omitempty"`
}

func (h handler) Exchange(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rewardID := chi.URLParam(r, "id")

	idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	prof, err := getProfile(idToken)
	if err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	rewardSnap, err := config.DataCollection(h.fs, "reward_catalog").Doc(rewardID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			presenter.BadRequest(w, "特典が見つかりません")
			return
		}
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var reward rewardCatalogDoc
	if err := rewardSnap.DataTo(&reward); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}
	if !reward.Active {
		presenter.BadRequest(w, "この特典は現在ご利用いただけません")
		return
	}

	memberRef := config.DataCollection(h.fs, "members").Doc(prof.UserID)
	var couponID string
	var newPoint int
	var squareCode, productURL string

	err = h.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// ── 全 read ──────────────────────────────────────────────
		// Firestore トランザクションは read → write の順が必須。
		// pool の検索と member の取得をまとめて先に行う。

		var candidate *square.PoolCandidate
		if reward.PricingRuleID != "" {
			var findErr error
			candidate, findErr = h.pool.FindInTx(tx, reward.PricingRuleID)
			if findErr != nil && !errors.Is(findErr, square.ErrPoolEmpty) {
				return findErr
			}
			if errors.Is(findErr, square.ErrPoolEmpty) {
				logger.Error("square pool empty for pricing_rule_id: " + reward.PricingRuleID)
			}
		}

		memberSnap, err := tx.Get(memberRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return errors.New("メンバー情報が見つかりません")
			}
			return err
		}
		var m memberDocExchange
		if err := memberSnap.DataTo(&m); err != nil {
			return err
		}
		if m.Point < reward.RequiredPoints {
			return errInsufficientPoints
		}
		newPoint = m.Point - reward.RequiredPoints

		// ── 全 write ─────────────────────────────────────────────

		if candidate != nil {
			squareCode = candidate.Code
			if reward.SquareItemID != "" {
				productURL = fmt.Sprintf(
					"https://food-records.square.site/?item=%s&cc=%s",
					reward.SquareItemID, candidate.Code,
				)
			}
			if err := h.pool.ClaimInTx(tx, candidate, prof.UserID); err != nil {
				return err
			}
		}

		if err := tx.Update(memberRef, []firestore.Update{{Path: "point", Value: newPoint}}); err != nil {
			return err
		}

		couponRef := memberRef.Collection("coupons").NewDoc()
		couponID = couponRef.ID
		return tx.Set(couponRef, memberCouponDoc{
			Title:               reward.Title,
			Description:         reward.Description,
			ImageURL:            reward.ImageURL,
			RewardID:            rewardID,
			PointCost:           reward.RequiredPoints,
			IssuedAt:            time.Now(),
			ExpiresAt:           time.Now().AddDate(0, 3, 0),
			Used:                false,
			SquareDiscountCode:  squareCode,
			ProductURL:          productURL,
			BenefitType:         reward.BenefitType,
			TargetType:          reward.TargetType,
			MaxUnitPrice:        reward.MaxUnitPrice,
			FreeQuantity:        reward.FreeQuantity,
			EligibleStoreIDs:    reward.EligibleStoreIDs,
			EligibleCategoryIDs: reward.EligibleCategoryIDs,
			EligibleItemIDs:     reward.EligibleItemIDs,
			EligibleOptionIDs:   reward.EligibleOptionIDs,
			Status:              "available",
		})
	})

	if err != nil {
		if errors.Is(err, errInsufficientPoints) {
			presenter.BadRequest(w, fmt.Sprintf("ポイントが不足しています（必要: %dpt）", reward.RequiredPoints))
			return
		}
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	presenter.EncodeWithMessage(w, ExchangeResp{
		CouponID:           couponID,
		Title:              reward.Title,
		PointCost:          reward.RequiredPoints,
		NewPoint:           newPoint,
		SquareDiscountCode: squareCode,
		ProductURL:         productURL,
	})
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
