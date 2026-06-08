package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type pageToken struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeCursor(createdAt pgtype.Timestamptz, id pgtype.UUID) (string, error) {
	token := pageToken{
		CreatedAt: createdAt.Time,
		ID:        id.String(),
	}
	b, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func decodeCursor(token string) (createdAt pgtype.Timestamptz, id pgtype.UUID, err error) {
	b, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return createdAt, id, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var pt pageToken
	if err := json.Unmarshal(b, &pt); err != nil {
		return createdAt, id, fmt.Errorf("invalid cursor format: %w", err)
	}
	if err := createdAt.Scan(pt.CreatedAt); err != nil {
		return createdAt, id, fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	if err := id.Scan(pt.ID); err != nil {
		return createdAt, id, fmt.Errorf("invalid cursor id: %w", err)
	}
	return createdAt, id, nil
}
