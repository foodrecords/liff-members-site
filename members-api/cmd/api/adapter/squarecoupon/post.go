package squarecoupon

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/square"
)

type createRequest struct {
	Code                      string    `json:"code"`
	Name                      string    `json:"name"`
	PricingRuleToken          string    `json:"pricing_rule_token"` // 省略時は SQUARE_PRICING_RULE_TOKEN 環境変数を使用
	ExpiresAt                 time.Time `json:"expires_at"`
	MaxRedemptions            int       `json:"max_redemptions"`
	MaxRedemptionsPerCustomer int       `json:"max_redemptions_per_customer"`
}

func (h handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		presenter.BadRequest(w, "invalid request body")
		return
	}

	if req.Code == "" || req.Name == "" || req.PricingRuleToken == "" {
		presenter.BadRequest(w, "code・name・pricing_rule_token は必須です")
		return
	}
	if req.MaxRedemptions == 0 {
		req.MaxRedemptions = -1
	}
	if req.MaxRedemptionsPerCustomer == 0 {
		req.MaxRedemptionsPerCustomer = -1
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().AddDate(0, 1, 0)
	}

	sq, err := h.sq.Get()
	if err != nil {
		logger.Error("square session: " + err.Error())
		presenter.Error(w, err)
		return
	}

	result, err := sq.CreateCoupon(square.CreateCouponRequest{
		Code:                      req.Code,
		Name:                      req.Name,
		PricingRuleToken:          req.PricingRuleToken,
		ExpiresAt:                 req.ExpiresAt,
		MaxRedemptions:            req.MaxRedemptions,
		MaxRedemptionsPerCustomer: req.MaxRedemptionsPerCustomer,
	})
	if err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	presenter.EncodeWithMessage(w, result)
}
