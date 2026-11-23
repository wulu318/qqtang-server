package persistence

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// InviteCode is one registration ticket. Codes are single-use and bound to the
// account that consumed them.
type InviteCode struct {
	Code       string
	CreatedUTC string
	UsedBy     uint32
	UsedUTC    string
}

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newInviteCode() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, value := range buffer {
		builder.WriteByte(inviteAlphabet[int(value)%len(inviteAlphabet)])
	}
	return builder.String(), nil
}

// CreateInviteCodes returns freshly generated unused codes.
func (store *PlayerStore) CreateInviteCodes(ctx context.Context, count int) ([]string, error) {
	if count < 1 || count > 200 {
		return nil, fmt.Errorf("邀请码数量必须在 1 到 200 之间")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	codes := make([]string, 0, count)
	for len(codes) < count {
		code, genErr := newInviteCode()
		if genErr != nil {
			return nil, genErr
		}
		if _, execErr := tx.ExecContext(ctx, `INSERT INTO invite_codes(code, created_utc) VALUES(?, ?)`, code, now); execErr != nil {
			continue
		}
		codes = append(codes, code)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}

func (store *PlayerStore) ListInviteCodes(ctx context.Context, limit int) ([]InviteCode, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := store.db.QueryContext(ctx,
		`SELECT code, created_utc, used_by, used_utc FROM invite_codes ORDER BY created_utc DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codes := make([]InviteCode, 0, limit)
	for rows.Next() {
		var entry InviteCode
		var usedBy sql.NullInt64
		var usedUTC sql.NullString
		if err = rows.Scan(&entry.Code, &entry.CreatedUTC, &usedBy, &usedUTC); err != nil {
			return nil, err
		}
		if usedBy.Valid {
			entry.UsedBy = uint32(usedBy.Int64)
		}
		entry.UsedUTC = usedUTC.String
		codes = append(codes, entry)
	}
	return codes, rows.Err()
}

// ConsumeInviteCode atomically burns a code for one account. It returns
// ErrInviteInvalid when the code is unknown or already used.
var ErrInviteInvalid = errors.New("邀请码无效或已被使用")

func (store *PlayerStore) ConsumeInviteCode(ctx context.Context, code string, uin uint32) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || uin == 0 {
		return ErrInviteInvalid
	}
	result, err := store.db.ExecContext(ctx,
		`UPDATE invite_codes SET used_by = ?, used_utc = ? WHERE code = ? AND used_by IS NULL`,
		uin, time.Now().UTC().Format(time.RFC3339Nano), code)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrInviteInvalid
	}
	return nil
}
