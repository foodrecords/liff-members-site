package rank

// ランク閾値（累積ポイント）— 変更する場合はここだけ修正
const (
	BronzeThreshold = 1000
	SilverThreshold = 3000
	GoldThreshold   = 8000
	SecretThreshold = 20000
)

const (
	Green  = "green"
	Bronze = "bronze"
	Silver = "silver"
	Gold   = "gold"
	Secret = "secret" // フロントエンド非公開・GOLDとして表示
)

type Info struct {
	Current       string
	Next          string
	NextThreshold int // 次ランクまでの残りポイント（フロントエンド表示用）
}

func Calc(point int) Info {
	switch {
	case point >= SecretThreshold:
		return Info{Current: Secret}
	case point >= GoldThreshold:
		// Secret は非公開なので next_rank / next_rank_point は返さない
		return Info{Current: Gold}
	case point >= SilverThreshold:
		return Info{Current: Silver, Next: Gold, NextThreshold: GoldThreshold - point}
	case point >= BronzeThreshold:
		return Info{Current: Bronze, Next: Silver, NextThreshold: SilverThreshold - point}
	default:
		return Info{Current: Green, Next: Bronze, NextThreshold: BronzeThreshold - point}
	}
}
