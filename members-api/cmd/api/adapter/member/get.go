package member

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/rank"
	"github.com/foodrecords/members-api/pkg/square"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const twoYears = 2 * 365 * 24 * time.Hour

type memberDoc struct {
	Number            int64     `firestore:"number"`
	Name              string    `firestore:"name"`
	Point             int       `firestore:"point"`
	TotalEarnedPoint  int       `firestore:"total_earned_point"`
	LastAccumulatedAt time.Time `firestore:"last_accumulated_at,omitempty"`
}

type nextExpiry struct {
	Amount    int    `json:"amount"`
	ExpiresAt string `json:"expires_at"`
}

type GetResp struct {
	Number             string      `json:"number"`
	Name               string      `json:"name"`
	Point              int         `json:"point"`
	TotalEarnedPoint   int         `json:"total_earned_point"`
	Rank               string      `json:"rank"`
	NextRank           string      `json:"next_rank,omitempty"`
	NextRankPoint      int         `json:"next_rank_point,omitempty"`
	IsNewMember        bool        `json:"is_new_member,omitempty"`
	NextPointExpiry    *nextExpiry `json:"next_point_expiry,omitempty"`
	AccumulatedResetAt string      `json:"accumulated_reset_at,omitempty"`
}

type welcomeRewardDoc struct {
	Title          string `firestore:"title"`
	Description    string `firestore:"description"`
	ImageURL       string `firestore:"image_url"`
	RequiredPoints int    `firestore:"required_points"`
	PricingRuleID  string `firestore:"pricing_rule_id"`
	SquareItemID   string `firestore:"square_item_id"`
}

type couponDoc struct {
	Title              string    `firestore:"title"`
	Description        string    `firestore:"description"`
	ImageURL           string    `firestore:"image_url"`
	RewardID           string    `firestore:"reward_id"`
	PointCost          int       `firestore:"point_cost"`
	IssuedAt           time.Time `firestore:"issued_at"`
	ExpiresAt          time.Time `firestore:"expires_at"`
	Used               bool      `firestore:"used"`
	SquareDiscountCode string    `firestore:"square_discount_code,omitempty"`
	ProductURL         string    `firestore:"product_url,omitempty"`
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
	doc, err := memberRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var resp GetResp
	if doc != nil && doc.Exists() {
		var m memberDoc
		if err := doc.DataTo(&m); err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}

		// 最終累積加算から2年以上経過していた場合は total_earned_point をリセット
		if !m.LastAccumulatedAt.IsZero() && time.Since(m.LastAccumulatedAt) >= twoYears {
			if _, err := memberRef.Update(ctx, []firestore.Update{
				{Path: "total_earned_point", Value: 0},
			}); err != nil {
				logger.Error("reset total_earned_point: " + err.Error())
			}
			m.TotalEarnedPoint = 0
		}

		totalEarned := effectiveTotalEarned(m)
		resp.Number = fmt.Sprintf("FR%06d", m.Number)
		resp.Name = m.Name
		resp.Point = m.Point
		resp.TotalEarnedPoint = totalEarned
		ri := rank.Calc(totalEarned)
		resp.Rank = ri.Current
		resp.NextRank = ri.Next
		resp.NextRankPoint = ri.NextThreshold

		// 最も近いポイント失効予定を取得
		logDocs, logErr := memberRef.Collection("point_logs").
			Where("expires_at", ">", time.Now()).
			OrderBy("expires_at", firestore.Asc).
			Limit(1).
			Documents(ctx).GetAll()
		if logErr == nil && len(logDocs) > 0 {
			var pl struct {
				Amount    int       `firestore:"amount"`
				ExpiresAt time.Time `firestore:"expires_at"`
			}
			if logDocs[0].DataTo(&pl) == nil {
				resp.NextPointExpiry = &nextExpiry{
					Amount:    pl.Amount,
					ExpiresAt: pl.ExpiresAt.Format(time.RFC3339),
				}
			}
		}

		// 累積リセット予定日（最終累積加算 + 2年）
		if !m.LastAccumulatedAt.IsZero() {
			resp.AccumulatedResetAt = m.LastAccumulatedAt.Add(twoYears).Format(time.RFC3339)
		}
	} else {
		newNumber, err := generateUniqueNumber(ctx, h.fs)
		if err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		m := memberDoc{
			Number:           newNumber,
			Name:             prof.DisplayName,
			Point:            0,
			TotalEarnedPoint: 0,
		}
		batch := h.fs.Batch()
		batch.Set(memberRef, m)
		batch.Set(h.fs.Collection("member_numbers").Doc(fmt.Sprintf("%06d", newNumber)), map[string]any{"user_id": prof.UserID})

		// ウェルカムクーポン発行（最もポイントの少ないアクティブな特典を動的取得）
		if rewardSnaps, rewardErr := h.fs.Collection("reward_catalog").Where("active", "==", true).Documents(ctx).GetAll(); rewardErr == nil {
			var bestSnap *firestore.DocumentSnapshot
			var best welcomeRewardDoc
			for _, snap := range rewardSnaps {
				var wr welcomeRewardDoc
				if snap.DataTo(&wr) == nil && (bestSnap == nil || wr.RequiredPoints < best.RequiredPoints) {
					bestSnap, best = snap, wr
				}
			}
			if bestSnap != nil {
				var squareCode, productURL string
				if best.PricingRuleID != "" {
					code, err := h.pool.ClaimOne(ctx, best.PricingRuleID, prof.UserID)
					if err != nil && !errors.Is(err, square.ErrPoolEmpty) {
						logger.Error("welcome coupon: pool claim failed: " + err.Error())
					} else if err == nil {
						squareCode = code
						if best.SquareItemID != "" {
							productURL = fmt.Sprintf("https://food-records.square.site/?item=%s&cc=%s", best.SquareItemID, code)
						}
					}
				}
				now := time.Now()
				batch.Set(memberRef.Collection("coupons").NewDoc(), couponDoc{
					Title:              best.Title,
					Description:        best.Description,
					ImageURL:           best.ImageURL,
					RewardID:           bestSnap.Ref.ID,
					PointCost:          0,
					IssuedAt:           now,
					ExpiresAt:          now.AddDate(0, 3, 0),
					Used:               false,
					SquareDiscountCode: squareCode,
					ProductURL:         productURL,
				})
			} else {
				logger.Error("welcome coupon: no active reward found in reward_catalog")
			}
		} else {
			logger.Error("welcome coupon fetch: " + rewardErr.Error())
		}

		if _, err := batch.Commit(ctx); err != nil {
			logger.Error(err.Error())
			presenter.BadRequest(w, "FAILED_CREATE_USER")
			return
		}
		resp.Number = fmt.Sprintf("FR%06d", newNumber)
		resp.Name = prof.DisplayName
		resp.Point = 0
		resp.TotalEarnedPoint = 0
		resp.IsNewMember = true
		ri := rank.Calc(0)
		resp.Rank = ri.Current
		resp.NextRank = ri.Next
		resp.NextRankPoint = ri.NextThreshold
	}

	presenter.EncodeWithMessage(w, resp)
}

// effectiveTotalEarned は total_earned_point を返す。
// フィールド未設定の旧メンバーは point の値をフォールバックとして使用する。
func effectiveTotalEarned(m memberDoc) int {
	if m.TotalEarnedPoint == 0 && m.Point > 0 {
		return m.Point
	}
	return m.TotalEarnedPoint
}

func generateUniqueNumber(ctx context.Context, fs *firestore.Client) (int64, error) {
	ra := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		n := int64(ra.Int31n(1000000))
		doc, err := fs.Collection("member_numbers").Doc(fmt.Sprintf("%06d", n)).Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return n, nil
			}
			return 0, err
		}
		if !doc.Exists() {
			return n, nil
		}
	}
}

type GetProfileResp struct {
	UserID        string `json:"userId"`
	DisplayName   string `json:"displayName"`
	PictureURL    string `json:"pictureUrl"`
	StatusMessage string `json:"statusMessage"`
}

func getProfile(token string) (*GetProfileResp, error) {
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

	var profile GetProfileResp
	if err := json.Unmarshal(b, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}
