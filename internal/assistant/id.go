package assistant

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

func NewID(prefix string, now time.Time) string {
	entropy := make([]byte, 6)
	if _, err := rand.Read(entropy); err != nil {
		panic(fmt.Errorf("assistant: generate id entropy: %w", err))
	}
	return fmt.Sprintf("%s_%s_%s", prefix, now.UTC().Format("20060102T150405Z"), hex.EncodeToString(entropy))
}

func NewChatID(now time.Time) string {
	entropy := make([]byte, 5)
	if _, err := rand.Read(entropy); err != nil {
		panic(fmt.Errorf("assistant: generate chat id entropy: %w", err))
	}
	return fmt.Sprintf("chat_%s_%s", strconv.FormatInt(now.UTC().Unix(), 36), hex.EncodeToString(entropy))
}
