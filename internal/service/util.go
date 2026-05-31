package service

import (
	"encoding/binary"

	"github.com/jackc/pgx/v5/pgtype"
)

func uuidToInt64(id pgtype.UUID) int64 {
	return int64(binary.BigEndian.Uint64(id.Bytes[:8]))
}
