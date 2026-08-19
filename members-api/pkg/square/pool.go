package square

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
)

var ErrPoolEmpty = errors.New("square coupon pool is empty")

type poolEntry struct {
	Code string `firestore:"code"`
}

// PoolCandidate はトランザクション内で見つかった未使用コードの情報を保持します。
// Firestore トランザクションは全 read → 全 write の順番が必須なので、
// read 結果をこの構造体に保持し、write は ClaimInTx で行います。
type PoolCandidate struct {
	ref  *firestore.DocumentRef
	Code string
}

// Pool manages pre-generated discount codes stored in the Firestore square_pool collection.
type Pool struct {
	fs *firestore.Client
}

func NewPool(fs *firestore.Client) *Pool {
	return &Pool{fs: fs}
}

// FindInTx は未使用コードを1件検索します（read のみ）。
// トランザクション内の全 read が終わった後に ClaimInTx で確定してください。
func (p *Pool) FindInTx(tx *firestore.Transaction, pricingRuleID string) (*PoolCandidate, error) {
	docs, err := tx.Documents(
		p.fs.Collection("square_pool").
			Where("pricing_rule_id", "==", pricingRuleID).
			Where("used", "==", false).
			Limit(1),
	).GetAll()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrPoolEmpty
	}

	var entry poolEntry
	if err := docs[0].DataTo(&entry); err != nil {
		return nil, err
	}

	return &PoolCandidate{ref: docs[0].Ref, Code: entry.Code}, nil
}

// ClaimInTx はコードを使用済みにマークします（write のみ）。
// 必ず FindInTx および他の全 read の後に呼んでください。
func (p *Pool) ClaimInTx(tx *firestore.Transaction, c *PoolCandidate, userID string) error {
	return tx.Update(c.ref, []firestore.Update{
		{Path: "used", Value: true},
		{Path: "used_by", Value: userID},
		{Path: "used_at", Value: time.Now()},
	})
}

// ClaimOne atomically claims one unused code for the given pricing_rule_id.
// Returns ErrPoolEmpty if no codes are available.
func (p *Pool) ClaimOne(ctx context.Context, pricingRuleID, userID string) (string, error) {
	var code string
	err := p.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		candidate, err := p.FindInTx(tx, pricingRuleID)
		if err != nil {
			return err
		}
		code = candidate.Code
		return p.ClaimInTx(tx, candidate, userID)
	})
	return code, err
}

// CountAvailable returns the number of unused codes for the given pricing_rule_id.
func (p *Pool) CountAvailable(ctx context.Context, pricingRuleID string) (int, error) {
	docs, err := p.fs.Collection("square_pool").
		Where("pricing_rule_id", "==", pricingRuleID).
		Where("used", "==", false).
		Documents(ctx).GetAll()
	if err != nil {
		return 0, err
	}
	return len(docs), nil
}
