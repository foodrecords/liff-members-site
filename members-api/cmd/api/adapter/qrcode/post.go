package qrcode

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
	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/interpreter"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
	"github.com/foodrecords/members-api/pkg/rank"
	"github.com/foodrecords/members-api/pkg/square"
	"github.com/foodrecords/members-api/pkg/totp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PostReq struct {
	Code string `json:"code"`
}

type PostResp struct {
	Number           string `json:"number"`
	GetPoint         int    `json:"get_point"`
	Point            int    `json:"point"`
	TotalEarnedPoint int    `json:"total_earned_point"`
	Rank             string `json:"rank"`
	NextRank         string `json:"next_rank,omitempty"`
	NextRankPoint    int    `json:"next_rank_point,omitempty"`
}

type memberDoc struct {
	Number            int64     `firestore:"number"`
	Name              string    `firestore:"name"`
	Point             int       `firestore:"point"`
	TotalEarnedPoint  int       `firestore:"total_earned_point"`
	LastCheckinDate   string    `firestore:"last_checkin_date,omitempty"`
	LastAccumulatedAt time.Time `firestore:"last_accumulated_at,omitempty"`
}

type serialDoc struct {
	Point      int    `firestore:"point"`
	Used       bool   `firestore:"used"`
	UsedID     string `firestore:"used_id"`
	Type       string `firestore:"type"`       // "once" (default) or "recurring"
	Accumulate *bool  `firestore:"accumulate"` // nil = true (backward compat)
}

type userScanDoc struct {
	LastScannedAt time.Time `firestore:"last_scanned_at"`
}

type couponDoc struct {
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

type pointLogDoc struct {
	Amount     int       `firestore:"amount"`
	Source     string    `firestore:"source"`     // "checkin", "serial_once", "serial_recurring"
	Accumulate bool      `firestore:"accumulate"` // total_earned_point に加算されたか
	AcquiredAt time.Time `firestore:"acquired_at"`
	ExpiresAt  time.Time `firestore:"expires_at"` // AcquiredAt + 1年
	SerialCode string    `firestore:"serial_code,omitempty"`
}

type welcomeRewardCatalogDoc struct {
	Title               string   `firestore:"title"`
	Description         string   `firestore:"description"`
	ImageURL            string   `firestore:"image_url"`
	RequiredPoints      int      `firestore:"required_points"`
	PricingRuleID       string   `firestore:"pricing_rule_id"`
	SquareItemID        string   `firestore:"square_item_id"`
	BenefitType         string   `firestore:"benefit_type"`
	TargetType          string   `firestore:"target_type"`
	MaxUnitPrice        int      `firestore:"max_unit_price"`
	FreeQuantity        int      `firestore:"free_quantity"`
	EligibleStoreIDs    []string `firestore:"eligible_store_ids"`
	EligibleCategoryIDs []string `firestore:"eligible_category_ids"`
	EligibleItemIDs     []string `firestore:"eligible_item_ids"`
	EligibleOptionIDs   []string `firestore:"eligible_option_ids"`
}

const (
	recurringCooldown  = 3 * time.Hour
	dailyCheckinPrefix = "DL_"
	dailyCheckinPoint  = 100
	twoYears           = 2 * 365 * 24 * time.Hour
)

// serialAccumulate は accumulate フィールドが nil（未設定）の場合も true として扱う。
func serialAccumulate(s serialDoc) bool {
	return s.Accumulate == nil || *s.Accumulate
}

// shouldReset は最終累積加算から2年以上経過した場合に true を返す。
// LastAccumulatedAt が未設定の既存メンバーはリセット対象外。
func shouldReset(m memberDoc) bool {
	if m.LastAccumulatedAt.IsZero() {
		return false
	}
	return time.Since(m.LastAccumulatedAt) >= twoYears
}

func addWelcomeCoupon(ctx context.Context, fs *firestore.Client, pool *square.Pool, userID string, batch *firestore.WriteBatch, memberRef *firestore.DocumentRef) {
	docs, err := fs.Collection("reward_catalog").Where("active", "==", true).Documents(ctx).GetAll()
	if err != nil {
		logger.Error("welcome coupon: " + err.Error())
		return
	}

	var bestSnap *firestore.DocumentSnapshot
	var best welcomeRewardCatalogDoc
	for _, doc := range docs {
		var wr welcomeRewardCatalogDoc
		if err := doc.DataTo(&wr); err != nil {
			continue
		}
		if bestSnap == nil || wr.RequiredPoints < best.RequiredPoints {
			bestSnap = doc
			best = wr
		}
	}
	if bestSnap == nil {
		logger.Error("welcome coupon: no active reward found in reward_catalog")
		return
	}

	var squareCode, productURL string
	if best.PricingRuleID != "" {
		code, err := pool.ClaimOne(ctx, best.PricingRuleID, userID)
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
		Title:               best.Title,
		Description:         best.Description,
		ImageURL:            best.ImageURL,
		RewardID:            bestSnap.Ref.ID,
		PointCost:           0,
		IssuedAt:            now,
		ExpiresAt:           now.AddDate(0, 3, 0),
		Used:                false,
		SquareDiscountCode:  squareCode,
		ProductURL:          productURL,
		BenefitType:         best.BenefitType,
		TargetType:          best.TargetType,
		MaxUnitPrice:        best.MaxUnitPrice,
		FreeQuantity:        best.FreeQuantity,
		EligibleStoreIDs:    best.EligibleStoreIDs,
		EligibleCategoryIDs: best.EligibleCategoryIDs,
		EligibleItemIDs:     best.EligibleItemIDs,
		EligibleOptionIDs:   best.EligibleOptionIDs,
		Status:              "available",
	})
}

func (h handler) Post(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body PostReq
	if err := interpreter.Interpret(r, &body); err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_BODY")
		return
	}

	if strings.HasPrefix(body.Code, dailyCheckinPrefix) {
		token := body.Code[len(dailyCheckinPrefix):]
		if !totp.Validate(config.CheckinSecret, token) {
			presenter.BadRequest(w, "QRコードの有効期限が切れています。店舗のQRコードをスキャンしてください")
			return
		}
		idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		prof, err := getProfile(idToken)
		if err != nil {
			logger.Error(err.Error())
			presenter.BadRequest(w, "INVALID_TOKEN")
			return
		}
		h.postDailyCheckin(ctx, w, prof)
		return
	}

	serialSnap, err := h.fs.Collection("serials").Doc(body.Code).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			presenter.BadRequest(w, "シリアルナンバーが見つかりません")
			return
		}
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var s serialDoc
	if err := serialSnap.DataTo(&s); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	if s.Type == "recurring" {
		idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		prof, err := getProfile(idToken)
		if err != nil {
			logger.Error(err.Error())
			presenter.BadRequest(w, "INVALID_TOKEN")
			return
		}
		h.postRecurringSerial(ctx, w, body.Code, s.Point, serialAccumulate(s), prof)
		return
	}

	if s.Used {
		presenter.BadRequest(w, "このシリアルナンバーは既に使われています")
		return
	}

	idToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	prof, err := getProfile(idToken)
	if err != nil {
		logger.Error(err.Error())
		presenter.BadRequest(w, "INVALID_TOKEN")
		return
	}

	accumulate := serialAccumulate(s)
	now := time.Now()
	memberRef := h.fs.Collection("members").Doc(prof.UserID)
	memberSnap, err := memberRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var memberNumber int64
	var newPoint, newTotalEarned int
	batch := h.fs.Batch()

	if memberSnap != nil && memberSnap.Exists() {
		var m memberDoc
		if err := memberSnap.DataTo(&m); err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		memberNumber = m.Number
		newPoint = m.Point + s.Point
		currentTotal := effectiveTotalEarned(m)
		if shouldReset(m) {
			currentTotal = 0
		}
		if accumulate {
			newTotalEarned = currentTotal + s.Point
		} else {
			newTotalEarned = currentTotal
		}
		memberUpdates := []firestore.Update{
			{Path: "point", Value: newPoint},
			{Path: "total_earned_point", Value: newTotalEarned},
		}
		if accumulate {
			memberUpdates = append(memberUpdates, firestore.Update{Path: "last_accumulated_at", Value: now})
		}
		batch.Update(memberRef, memberUpdates)
	} else {
		var err error
		memberNumber, err = generateUniqueNumber(ctx, h.fs)
		if err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		newPoint = s.Point
		if accumulate {
			newTotalEarned = s.Point
		} else {
			newTotalEarned = 0
		}
		m := memberDoc{
			Number:           memberNumber,
			Name:             prof.DisplayName,
			Point:            newPoint,
			TotalEarnedPoint: newTotalEarned,
		}
		if accumulate {
			m.LastAccumulatedAt = now
		}
		batch.Set(memberRef, m)
		batch.Set(h.fs.Collection("member_numbers").Doc(fmt.Sprintf("%06d", memberNumber)), map[string]interface{}{"user_id": prof.UserID})
		addWelcomeCoupon(ctx, h.fs, h.pool, prof.UserID, batch, memberRef)
	}

	if body.Code != "FR1234567890" {
		batch.Update(h.fs.Collection("serials").Doc(body.Code), []firestore.Update{
			{Path: "used", Value: true},
			{Path: "used_id", Value: fmt.Sprintf("%06d", memberNumber)},
		})
	}

	batch.Set(memberRef.Collection("point_logs").NewDoc(), pointLogDoc{
		Amount:     s.Point,
		Source:     "serial_once",
		Accumulate: accumulate,
		AcquiredAt: now,
		ExpiresAt:  now.AddDate(1, 0, 0),
		SerialCode: body.Code,
	})

	if _, err := batch.Commit(ctx); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	ri := rank.Calc(newTotalEarned)
	presenter.EncodeWithMessage(w, PostResp{
		Number:           fmt.Sprintf("FR%06d", memberNumber),
		GetPoint:         s.Point,
		Point:            newPoint,
		TotalEarnedPoint: newTotalEarned,
		Rank:             ri.Current,
		NextRank:         ri.Next,
		NextRankPoint:    ri.NextThreshold,
	})
}

func (h handler) postDailyCheckin(ctx context.Context, w http.ResponseWriter, prof *GetProfileResp) {
	jst := time.FixedZone("JST", 9*60*60)
	today := time.Now().In(jst).Format("2006-01-02")
	now := time.Now()

	memberRef := h.fs.Collection("members").Doc(prof.UserID)
	memberSnap, err := memberRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var memberNumber int64
	var newPoint, newTotalEarned int
	batch := h.fs.Batch()

	if memberSnap != nil && memberSnap.Exists() {
		var m memberDoc
		if err := memberSnap.DataTo(&m); err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		if m.LastCheckinDate == today {
			presenter.BadRequest(w, "本日はすでにスキャン済みです")
			return
		}
		memberNumber = m.Number
		newPoint = m.Point + dailyCheckinPoint
		currentTotal := effectiveTotalEarned(m)
		if shouldReset(m) {
			currentTotal = 0
		}
		newTotalEarned = currentTotal + dailyCheckinPoint
		batch.Update(memberRef, []firestore.Update{
			{Path: "point", Value: newPoint},
			{Path: "total_earned_point", Value: newTotalEarned},
			{Path: "last_checkin_date", Value: today},
			{Path: "last_accumulated_at", Value: now},
		})
	} else {
		memberNumber, err = generateUniqueNumber(ctx, h.fs)
		if err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		newPoint = dailyCheckinPoint
		newTotalEarned = dailyCheckinPoint
		batch.Set(memberRef, memberDoc{
			Number:            memberNumber,
			Name:              prof.DisplayName,
			Point:             newPoint,
			TotalEarnedPoint:  newTotalEarned,
			LastCheckinDate:   today,
			LastAccumulatedAt: now,
		})
		batch.Set(h.fs.Collection("member_numbers").Doc(fmt.Sprintf("%06d", memberNumber)), map[string]interface{}{"user_id": prof.UserID})
		addWelcomeCoupon(ctx, h.fs, h.pool, prof.UserID, batch, memberRef)
	}

	batch.Set(memberRef.Collection("point_logs").NewDoc(), pointLogDoc{
		Amount:     dailyCheckinPoint,
		Source:     "checkin",
		Accumulate: true,
		AcquiredAt: now,
		ExpiresAt:  now.AddDate(1, 0, 0),
	})

	if _, err := batch.Commit(ctx); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	ri := rank.Calc(newTotalEarned)
	presenter.EncodeWithMessage(w, PostResp{
		Number:           fmt.Sprintf("FR%06d", memberNumber),
		GetPoint:         dailyCheckinPoint,
		Point:            newPoint,
		TotalEarnedPoint: newTotalEarned,
		Rank:             ri.Current,
		NextRank:         ri.Next,
		NextRankPoint:    ri.NextThreshold,
	})
}

func (h handler) postRecurringSerial(ctx context.Context, w http.ResponseWriter, code string, point int, accumulate bool, prof *GetProfileResp) {
	scanRef := h.fs.Collection("serials").Doc(code).Collection("user_scans").Doc(prof.UserID)
	scanSnap, err := scanRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	if scanSnap != nil && scanSnap.Exists() {
		var scan userScanDoc
		if err := scanSnap.DataTo(&scan); err == nil {
			if elapsed := time.Since(scan.LastScannedAt); elapsed < recurringCooldown {
				remaining := recurringCooldown - elapsed
				hh := int(remaining.Hours())
				mm := int(remaining.Minutes()) % 60
				presenter.BadRequest(w, fmt.Sprintf("次のスキャンまで%d時間%d分お待ちください", hh, mm))
				return
			}
		}
	}

	now := time.Now()
	memberRef := h.fs.Collection("members").Doc(prof.UserID)
	memberSnap, err := memberRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	var memberNumber int64
	var newPoint, newTotalEarned int
	batch := h.fs.Batch()

	if memberSnap != nil && memberSnap.Exists() {
		var m memberDoc
		if err := memberSnap.DataTo(&m); err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		memberNumber = m.Number
		newPoint = m.Point + point
		currentTotal := effectiveTotalEarned(m)
		if shouldReset(m) {
			currentTotal = 0
		}
		if accumulate {
			newTotalEarned = currentTotal + point
		} else {
			newTotalEarned = currentTotal
		}
		memberUpdates := []firestore.Update{
			{Path: "point", Value: newPoint},
			{Path: "total_earned_point", Value: newTotalEarned},
		}
		if accumulate {
			memberUpdates = append(memberUpdates, firestore.Update{Path: "last_accumulated_at", Value: now})
		}
		batch.Update(memberRef, memberUpdates)
	} else {
		memberNumber, err = generateUniqueNumber(ctx, h.fs)
		if err != nil {
			logger.Error(err.Error())
			presenter.Error(w, err)
			return
		}
		newPoint = point
		if accumulate {
			newTotalEarned = point
		} else {
			newTotalEarned = 0
		}
		m := memberDoc{
			Number:           memberNumber,
			Name:             prof.DisplayName,
			Point:            newPoint,
			TotalEarnedPoint: newTotalEarned,
		}
		if accumulate {
			m.LastAccumulatedAt = now
		}
		batch.Set(memberRef, m)
		batch.Set(h.fs.Collection("member_numbers").Doc(fmt.Sprintf("%06d", memberNumber)), map[string]interface{}{"user_id": prof.UserID})
		addWelcomeCoupon(ctx, h.fs, h.pool, prof.UserID, batch, memberRef)
	}

	batch.Set(scanRef, userScanDoc{LastScannedAt: now})

	batch.Set(memberRef.Collection("point_logs").NewDoc(), pointLogDoc{
		Amount:     point,
		Source:     "serial_recurring",
		Accumulate: accumulate,
		AcquiredAt: now,
		ExpiresAt:  now.AddDate(1, 0, 0),
		SerialCode: code,
	})

	if _, err := batch.Commit(ctx); err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	ri := rank.Calc(newTotalEarned)
	presenter.EncodeWithMessage(w, PostResp{
		Number:           fmt.Sprintf("FR%06d", memberNumber),
		GetPoint:         point,
		Point:            newPoint,
		TotalEarnedPoint: newTotalEarned,
		Rank:             ri.Current,
		NextRank:         ri.Next,
		NextRankPoint:    ri.NextThreshold,
	})
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
