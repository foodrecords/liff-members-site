package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/rank"
	"github.com/go-chi/chi"
)

type handler struct {
	store *Store
}

func getUserID(r *http.Request) (string, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return "", fmt.Errorf("missing token")
	}
	return token, nil
}

// GET /members
func (h *handler) getMember(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	member, isNew, err := h.store.GetOrCreateMember(userID)
	if err != nil {
		presenter.Error(w, err)
		return
	}

	totalEarned := effectiveTotalEarned(member)
	ri := rank.Calc(totalEarned)
	presenter.EncodeWithMessage(w, memberResp{
		Number:           fmt.Sprintf("FR%06d", member.Number),
		Name:             member.Name,
		Point:            member.Point,
		TotalEarnedPoint: totalEarned,
		Rank:             ri.Current,
		NextRank:         ri.Next,
		NextRankPoint:    ri.NextThreshold,
		IsNewMember:      isNew,
	})
}

type memberResp struct {
	Number           string `json:"number"`
	Name             string `json:"name"`
	Point            int    `json:"point"`
	TotalEarnedPoint int    `json:"total_earned_point"`
	Rank             string `json:"rank"`
	NextRank         string `json:"next_rank,omitempty"`
	NextRankPoint    int    `json:"next_rank_point,omitempty"`
	IsNewMember      bool   `json:"is_new_member,omitempty"`
}

// POST /qrcode
func (h *handler) postQRCode(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}

	// Daily checkin: any DL_ prefixed code triggers checkin (no TOTP in dev)
	if strings.HasPrefix(body.Code, "DL_") {
		result, err := h.store.AddPointFromCheckin(userID)
		if err != nil {
			if err == errAlreadyCheckedIn {
				presenter.BadRequest(w, "本日はすでにスキャン済みです")
				return
			}
			presenter.Error(w, err)
			return
		}
		ri := rank.Calc(result.NewTotalEarned)
		presenter.EncodeWithMessage(w, qrcodeResp{
			Number:           fmt.Sprintf("FR%06d", result.MemberNumber),
			GetPoint:         result.GetPoint,
			Point:            result.NewPoint,
			TotalEarnedPoint: result.NewTotalEarned,
			Rank:             ri.Current,
			NextRank:         ri.Next,
			NextRankPoint:    ri.NextThreshold,
		})
		return
	}

	result, err := h.store.AddPointFromSerial(userID, body.Code)
	if err != nil {
		switch err {
		case errNotFound:
			presenter.BadRequest(w, "シリアルナンバーが見つかりません")
		case errAlreadyUsed:
			presenter.BadRequest(w, "このシリアルナンバーは既に使われています")
		default:
			presenter.Error(w, err)
		}
		return
	}

	ri := rank.Calc(result.NewTotalEarned)
	presenter.EncodeWithMessage(w, qrcodeResp{
		Number:           fmt.Sprintf("FR%06d", result.MemberNumber),
		GetPoint:         result.GetPoint,
		Point:            result.NewPoint,
		TotalEarnedPoint: result.NewTotalEarned,
		Rank:             ri.Current,
		NextRank:         ri.Next,
		NextRankPoint:    ri.NextThreshold,
	})
}

type qrcodeResp struct {
	Number           string `json:"number"`
	GetPoint         int    `json:"get_point"`
	Point            int    `json:"point"`
	TotalEarnedPoint int    `json:"total_earned_point"`
	Rank             string `json:"rank"`
	NextRank         string `json:"next_rank,omitempty"`
	NextRankPoint    int    `json:"next_rank_point,omitempty"`
}

// GET /coupons
func (h *handler) getCoupons(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	memberCoupons := h.store.GetMemberCoupons(userID)

	coupons := make([]couponResp, 0, len(memberCoupons))
	for id, c := range memberCoupons {
		cr := couponResp{
			ID:             id,
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
		}
		if !c.ExpiresAt.IsZero() {
			cr.ExpiresAt = c.ExpiresAt.Format(time.RFC3339)
		}
		if c.Used && !c.UsedAt.IsZero() {
			cr.UsedAt = c.UsedAt.Format(time.RFC3339)
		}
		coupons = append(coupons, cr)
	}

	presenter.EncodeWithMessage(w, struct {
		Coupons []couponResp `json:"coupons"`
	}{Coupons: coupons})
}

type couponResp struct {
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
}

// POST /coupons/{id}/use
func (h *handler) useCoupon(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	couponID := chi.URLParam(r, "id")
	if err := h.store.UseCoupon(userID, couponID); err != nil {
		switch err {
		case errNotFound:
			presenter.BadRequest(w, "クーポンが見つかりません")
		case errAlreadyUsed:
			presenter.BadRequest(w, "このクーポンは既に使用済みです")
		default:
			presenter.Error(w, err)
		}
		return
	}
	presenter.Success(w)
}

// GET /rewards
func (h *handler) getRewards(w http.ResponseWriter, r *http.Request) {
	items := h.store.GetRewards()

	sort.Slice(items, func(i, j int) bool {
		if items[i].Doc.SortOrder != items[j].Doc.SortOrder {
			return items[i].Doc.SortOrder < items[j].Doc.SortOrder
		}
		return items[i].Doc.RequiredPoints < items[j].Doc.RequiredPoints
	})

	rewards := make([]rewardResp, 0, len(items))
	for _, item := range items {
		rewards = append(rewards, rewardResp{
			ID:             item.ID,
			Title:          item.Doc.Title,
			RequiredPoints: item.Doc.RequiredPoints,
			Description:    item.Doc.Description,
			ImageURL:       item.Doc.ImageURL,
			SortOrder:      item.Doc.SortOrder,
		})
	}
	presenter.EncodeWithMessage(w, rewards)
}

type rewardResp struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	RequiredPoints int    `json:"required_points"`
	Description    string `json:"description,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	SortOrder      int    `json:"sort_order"`
}

// POST /rewards/{id}/exchange
func (h *handler) exchangeReward(w http.ResponseWriter, r *http.Request) {
	userID, err := getUserID(r)
	if err != nil {
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	rewardID := chi.URLParam(r, "id")
	result, err := h.store.ExchangeReward(userID, rewardID)
	if err != nil {
		switch err {
		case errNotFound:
			presenter.BadRequest(w, "特典が見つかりません")
		case errInsufficientPoints:
			presenter.BadRequest(w, "ポイントが不足しています")
		default:
			presenter.Error(w, err)
		}
		return
	}

	presenter.EncodeWithMessage(w, struct {
		CouponID  string `json:"coupon_id"`
		Title     string `json:"title"`
		PointCost int    `json:"point_cost"`
		NewPoint  int    `json:"new_point"`
	}{
		CouponID:  result.CouponID,
		Title:     result.Title,
		PointCost: result.PointCost,
		NewPoint:  result.NewPoint,
	})
}

// GET /dev/users
func (h *handler) devListUsers(w http.ResponseWriter, r *http.Request) {
	presenter.EncodeWithMessage(w, h.store.ListMembers())
}

// GET /dev/serials
func (h *handler) devListSerials(w http.ResponseWriter, r *http.Request) {
	presenter.EncodeWithMessage(w, h.store.ListSerials())
}

// POST /dev/serials  body: {"code":"FRXXX","point":5}
func (h *handler) devCreateSerial(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code  string `json:"code"`
		Point int    `json:"point"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.Point <= 0 {
		presenter.BadRequest(w, "code と point(>0) が必要です")
		return
	}
	if err := h.store.CreateSerial(body.Code, body.Point); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.Success(w)
}

// POST /dev/reset
func (h *handler) devReset(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Reset(); err != nil {
		presenter.Error(w, err)
		return
	}
	presenter.Success(w)
}
