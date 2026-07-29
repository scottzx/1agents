package meta

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

type storeCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

func normalizePageLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultPageLimit, nil
	}
	if limit < 1 || limit > maxPageLimit {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidCursor, maxPageLimit)
	}
	return limit, nil
}

func encodeStoreCursor(at time.Time, id string) string {
	raw, _ := json.Marshal(storeCursor{At: timeToStr(at), ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeStoreCursor(value string) (storeCursor, error) {
	if value == "" {
		return storeCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return storeCursor{}, fmt.Errorf("%w: malformed base64", ErrInvalidCursor)
	}
	var c storeCursor
	if err := json.Unmarshal(raw, &c); err != nil || c.At == "" || c.ID == "" {
		return storeCursor{}, fmt.Errorf("%w: malformed payload", ErrInvalidCursor)
	}
	if strToTime(c.At).IsZero() {
		return storeCursor{}, fmt.Errorf("%w: invalid timestamp", ErrInvalidCursor)
	}
	return c, nil
}
