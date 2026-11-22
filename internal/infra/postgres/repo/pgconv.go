// Package repo содержит реализации портов application.*Repo поверх sqlcgen.
//
// Обёртки маппят sqlcgen.* в domain.* и инфраструктурные ошибки
// (pgx.ErrNoRows) в доменные (domain.ErrNotFound). Бизнес-логика не видит
// pgx и pgtype.
package repo

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// toUUID конвертирует pgtype.UUID в uuid.UUID.
// Невалидный (Valid == false) даёт uuid.Nil — в наших запросах UUID
// всегда NOT NULL, так что этот путь не достигается.
func toUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}
	return uuid.UUID(p.Bytes)
}

// toPgUUID конвертирует uuid.UUID в pgtype.UUID для параметров запроса.
func toPgUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(u), Valid: true}
}
