package poolmonitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/foodrecords/members-api/pkg/config"
	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
)

const defaultAlertThreshold = 20

type poolDoc struct {
	PricingRuleID string `firestore:"pricing_rule_id"`
}

type RuleStatus struct {
	Available int  `json:"available"`
	Alert     bool `json:"alert"`
}

func (h handler) Status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	docs, err := config.DataCollection(h.fs, "square_pool").
		Where("used", "==", false).
		Documents(ctx).GetAll()
	if err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	threshold := defaultAlertThreshold
	if v := os.Getenv("SQUARE_POOL_ALERT_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			threshold = n
		}
	}

	counts := make(map[string]int)
	for _, doc := range docs {
		var entry poolDoc
		if err := doc.DataTo(&entry); err != nil {
			continue
		}
		counts[entry.PricingRuleID]++
	}

	result := make(map[string]RuleStatus, len(counts))
	var alertLines []string
	total := 0
	for id, count := range counts {
		alert := count < threshold
		result[id] = RuleStatus{Available: count, Alert: alert}
		total += count
		if alert {
			alertLines = append(alertLines, fmt.Sprintf("• `%s`: %d 件", id, count))
		}
	}

	if len(alertLines) > 0 {
		msg := fmt.Sprintf(
			"⚠️ Square クーポンプール残数アラート\n%s\n\n補充手順:\n```\ncd square-session\nnpm run login:manual\nnpm run fill-pool\n```",
			strings.Join(alertLines, "\n"),
		)
		if err := sendDiscord(msg); err != nil {
			logger.Error("discord notify: " + err.Error())
		}
	} else if os.Getenv("DISCORD_WEBHOOK_URL") != "" {
		msg := fmt.Sprintf(
			"✅ Square クーポンプール正常 (%s)\n合計 %d 件のコードが使用可能",
			time.Now().In(jst()).Format("2006-01-02 15:04"),
			total,
		)
		if err := sendDiscord(msg); err != nil {
			logger.Error("discord notify: " + err.Error())
		}
	}

	presenter.EncodeWithMessage(w, result)
}

func jst() *time.Location {
	loc, _ := time.LoadLocation("Asia/Tokyo")
	return loc
}

func sendDiscord(msg string) error {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return nil
	}
	body, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		return err
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
