package reward

import (
	"net/http"
	"sort"

	"github.com/foodrecords/members-api/pkg/logger"
	"github.com/foodrecords/members-api/pkg/presenter"
)

type rewardCatalogDoc struct {
	Title               string   `firestore:"title"`
	RequiredPoints      int      `firestore:"required_points"`
	Description         string   `firestore:"description"`
	Active              bool     `firestore:"active"`
	ImageURL            string   `firestore:"image_url"`
	SortOrder           int      `firestore:"sort_order"`
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

type RewardResp struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	RequiredPoints int    `json:"required_points"`
	Description    string `json:"description,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	SortOrder      int    `json:"sort_order"`
}

func (h handler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	docs, err := h.fs.Collection("reward_catalog").Where("active", "==", true).Documents(ctx).GetAll()
	if err != nil {
		logger.Error(err.Error())
		presenter.Error(w, err)
		return
	}

	rewards := make([]RewardResp, 0, len(docs))
	for _, doc := range docs {
		var d rewardCatalogDoc
		if err := doc.DataTo(&d); err != nil {
			logger.Error(err.Error())
			continue
		}
		rewards = append(rewards, RewardResp{
			ID:             doc.Ref.ID,
			Title:          d.Title,
			RequiredPoints: d.RequiredPoints,
			Description:    d.Description,
			ImageURL:       d.ImageURL,
			SortOrder:      d.SortOrder,
		})
	}

	sort.Slice(rewards, func(i, j int) bool {
		if rewards[i].SortOrder != rewards[j].SortOrder {
			return rewards[i].SortOrder < rewards[j].SortOrder
		}
		return rewards[i].RequiredPoints < rewards[j].RequiredPoints
	})

	presenter.EncodeWithMessage(w, rewards)
}
