package square

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Client は Square のブラウザ内部APIを _sqweb_session Cookie で呼び出します。
// webSession は _sqweb_session Cookie の値（URLエンコード済みでもOK）。
// CSRFトークンはセッション値から自動抽出します。
// apiToken は公式 Square API (connect.squareup.com) の Bearer トークンです。
type Client struct {
	cookie     string
	csrfToken  string
	apiToken   string
	httpClient *http.Client
}

func NewClient(webSession, apiToken string) (*Client, error) {
	csrf, err := extractCSRF(webSession)
	if err != nil {
		return nil, fmt.Errorf("CSRFトークン抽出失敗: %w", err)
	}
	return &Client{
		cookie:     "_sqweb_session=" + webSession,
		csrfToken:  csrf,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// extractCSRF は _sqweb_session の値から _csrf_token を取り出します。
// セッション形式: <base64json>--<signature> (URLエンコードあり)
func extractCSRF(webSession string) (string, error) {
	decoded, err := url.QueryUnescape(webSession)
	if err != nil {
		decoded = webSession
	}
	parts := strings.SplitN(decoded, "--", 2)
	if len(parts) < 1 {
		return "", fmt.Errorf("セッション形式が不正です")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("base64デコード失敗: %w", err)
	}
	var payload struct {
		CSRFToken string `json:"_csrf_token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("JSONパース失敗: %w", err)
	}
	if payload.CSRFToken == "" {
		return "", fmt.Errorf("_csrf_token が見つかりません")
	}
	return payload.CSRFToken, nil
}

type CreateCouponRequest struct {
	Code                      string    `json:"code"`
	Name                      string    `json:"name"`
	PricingRuleToken          string    `json:"pricing_rule_token"`
	ExpiresAt                 time.Time `json:"expires_at"`
	MaxRedemptions            int       `json:"max_redemptions"`
	MaxRedemptionsPerCustomer int       `json:"max_redemptions_per_customer"`
}

type CreateCouponResponse struct {
	DiscountCodeID string    `json:"discount_code_id"`
	Code           string    `json:"code"`
	PricingRuleID  string    `json:"pricing_rule_id"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreateCoupon は以下の2ステップでクーポンを発行します。
//  1. app.squareup.com/api/v3/itemsfe/get で PRICING_RULE の現在バージョンを取得
//  2. connect.squareup.com/v2/discount-codes でディスカウントコードを作成
func (c *Client) CreateCoupon(req CreateCouponRequest) (*CreateCouponResponse, error) {
	version, err := c.getPricingRuleVersion(req.PricingRuleToken)
	if err != nil {
		return nil, fmt.Errorf("pricing rule version: %w", err)
	}

	codeID, createdAt, err := c.createDiscountCode(req.Code, req.PricingRuleToken, version, req.ExpiresAt, req.MaxRedemptions, req.MaxRedemptionsPerCustomer)
	if err != nil {
		return nil, fmt.Errorf("discount code: %w", err)
	}

	return &CreateCouponResponse{
		DiscountCodeID: codeID,
		Code:           req.Code,
		PricingRuleID:  req.PricingRuleToken,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      createdAt,
	}, nil
}

// --- Catalog API (公式) ---

type catalogObjectResp struct {
	Object struct {
		Version int64 `json:"version"`
	} `json:"object"`
	Errors []squareError `json:"errors"`
}

func (c *Client) getPricingRuleVersion(id string) (int64, error) {
	req, err := http.NewRequest(http.MethodGet, "https://connect.squareup.com/v2/catalog/object/"+id, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Square-Version", "2025-01-23")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("Square Catalog API %d: %s", resp.StatusCode, raw)
	}

	var result catalogObjectResp
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, err
	}
	if len(result.Errors) > 0 {
		return 0, fmt.Errorf("%s: %s", result.Errors[0].Code, result.Errors[0].Detail)
	}
	if result.Object.Version == 0 {
		return 0, fmt.Errorf("pricing rule %s が見つかりません", id)
	}
	return result.Object.Version, nil
}

// --- discount-codes (connect.squareup.com) ---

type createDiscountCodeReq struct {
	IdempotencyKey string       `json:"idempotency_key"`
	DiscountCode   discountCode `json:"discount_code"`
}

type discountCode struct {
	Code                      string `json:"code"`
	PricingRuleID             string `json:"pricing_rule_id"`
	MaxRedemptions            int    `json:"max_redemptions"`
	ExpiresAt                 string `json:"expires_at"`
	Reason                    string `json:"reason"`
	PricingRuleVersion        int64  `json:"pricing_rule_version"`
	MaxRedemptionsPerCustomer int    `json:"max_redemptions_per_customer"`
}

type createDiscountCodeResp struct {
	DiscountCode struct {
		ID        string    `json:"id"`
		Code      string    `json:"code"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"discount_code"`
	Errors []squareError `json:"errors"`
}

type squareError struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Detail   string `json:"detail"`
}

func (c *Client) createDiscountCode(code, pricingRuleToken string, version int64, expiresAt time.Time, maxRedemptions, maxPerCustomer int) (id string, createdAt time.Time, err error) {
	body := createDiscountCodeReq{
		IdempotencyKey: uuid.NewString(),
		DiscountCode: discountCode{
			Code:                      code,
			PricingRuleID:             pricingRuleToken,
			MaxRedemptions:            maxRedemptions,
			ExpiresAt:                 expiresAt.UTC().Format("2006-01-02T15:04:05.999Z"),
			Reason:                    "COUPON_BUILDER",
			PricingRuleVersion:        version,
			MaxRedemptionsPerCustomer: maxPerCustomer,
		},
	}

	var resp createDiscountCodeResp
	if err := c.postConnect("https://connect.squareup.com/v2/discount-codes", body, &resp); err != nil {
		return "", time.Time{}, err
	}
	if len(resp.Errors) > 0 {
		return "", time.Time{}, fmt.Errorf("%s: %s", resp.Errors[0].Code, resp.Errors[0].Detail)
	}
	return resp.DiscountCode.ID, resp.DiscountCode.CreatedAt, nil
}

// CreateOnceCode は max_redemptions=1 の1回限りクーポンコードを自動生成して発行します。
// 返り値は (生成コード, Square discount_code ID, error)
func (c *Client) CreateOnceCode(pricingRuleToken, name string, expiresAt time.Time) (code, discountCodeID string, err error) {
	code = generateMemberCode()
	resp, err := c.CreateCoupon(CreateCouponRequest{
		Code:                      code,
		Name:                      name,
		PricingRuleToken:          pricingRuleToken,
		ExpiresAt:                 expiresAt,
		MaxRedemptions:            1,
		MaxRedemptionsPerCustomer: 1,
	})
	if err != nil {
		return "", "", err
	}
	return code, resp.DiscountCodeID, nil
}

// generateMemberCode は "MBR" + 大文字英数字8文字 の形式でコードを生成します。
func generateMemberCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return "MBR" + string(b)
}

// --- HTTP helpers ---

func (c *Client) postConnect(rawURL string, reqBody, respBody any) error {
	return c.doPost(rawURL, reqBody, respBody, nil)
}

func (c *Client) doPost(rawURL string, reqBody, respBody any, extra map[string]string) error {
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("x-csrf-token", c.csrfToken)
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Origin", "https://app.squareup.com")
	req.Header.Set("Referer", "https://app.squareup.com/dashboard/customers/marketing/coupons/new")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")
	req.Header.Set("x-allow-cookies", ",C0001,C0002,C0003,C0004,")
	req.Header.Set("x-block-cookies", "true")
	for k, v := range extra {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Square %d: %s", resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, respBody)
}
