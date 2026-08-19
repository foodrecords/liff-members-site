package totp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"
)

const Window = 6 * 60 * 60 // 6-hour window (updates at 3:00/9:00/15:00/21:00 JST)
const tokenLen = 12

func Generate(secret string, t time.Time) string {
	w := uint64(t.Unix() / Window)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, w)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil))[:tokenLen]
}

// Validate accepts tokens from the current window ± 3 minutes.
// The display polls every 60s, so an old QR can linger up to 60s after a boundary.
// 3-minute tolerance provides margin for polling delay and JS timer imprecision.
func Validate(secret, token string) bool {
	now := time.Now()
	for _, delta := range []time.Duration{0, -3 * time.Minute, 3 * time.Minute} {
		if Generate(secret, now.Add(delta)) == token {
			return true
		}
	}
	return false
}
