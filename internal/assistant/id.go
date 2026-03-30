package assistant

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewID(prefix string, now time.Time) string {
	entropy := make([]byte, 6)
	if _, err := rand.Read(entropy); err != nil {
		panic(fmt.Errorf("assistant: generate id entropy: %w", err))
	}
	return fmt.Sprintf("%s_%s_%s", prefix, now.UTC().Format("20060102T150405Z"), hex.EncodeToString(entropy))
}
