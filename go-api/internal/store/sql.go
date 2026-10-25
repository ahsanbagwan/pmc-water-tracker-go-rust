package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"pmc/internal/model"
)

// SQL is the MySQL store. Table/column names match what Hibernate's default naming produced for
// the old Spring app (locality, app_user, audit_log; snake_case columns), so the Go service runs
// against the SAME Aiven database with no migration. It only needs a registered `mysql` driver
// (see cmd/api/driver_prod.go) - the package itself imports nothing outside the stdlib.
type SQL struct{ DB *sql.DB }

const DDL = `
CREATE TABLE IF NOT EXISTS locality (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(255) NOT NULL, ward VARCHAR(255), status VARCHAR(32) NOT NULL,
  latitude DOUBLE, longitude DOUBLE, notes VARCHAR(1000),
  source_name VARCHAR(500), source_url VARCHAR(500), last_verified DATE,
  verified BIT(1) NOT NULL, source_url_healthy BIT(1), source_url_last_checked DATETIME(6),
  citizen_description VARCHAR(1000), reported_urgency VARCHAR(16), urgency_score INT,
  urgency_signals VARCHAR(500), possible_duplicate_of_id BIGINT, duplicate_confidence DOUBLE
);
CREATE TABLE IF NOT EXISTS app_user (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(255) NOT NULL UNIQUE, password_hash VARCHAR(255) NOT NULL, role VARCHAR(16) NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(255) NOT NULL, action VARCHAR(255) NOT NULL, detail VARCHAR(1000),
  success BIT(1) NOT NULL, error_message VARCHAR(1000), ` + "`timestamp`" + ` DATETIME(6) NOT NULL
);`

func (s *SQL) Migrate(ctx context.Context) error {
	for _, stmt := range strings.Split(DDL, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// bit scans MySQL BIT(1), which the driver returns as []byte{0|1}.
type bit struct {
	V     bool
	Valid bool
}

func (b *bit) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		b.V, b.Valid = false, false
	case bool:
		b.V, b.Valid = v, true
	case int64:
		b.V, b.Valid = v != 0, true
	case []byte:
		b.V, b.Valid = len(v) > 0 && (v[0] == 1 || v[0] == '1'), true
	default:
		return fmt.Errorf("cannot scan %T into bit", src)
	}
	return nil
}

const locCols = `id, name, ward, status, latitude, longitude, notes, source_name, source_url, last_verified,
 verified, source_url_healthy, source_url_last_checked, citizen_description, reported_urgency, urgency_score,
 urgency_signals, possible_duplicate_of_id, duplicate_confidence`

type rowScanner interface{ Scan(...any) error }

func scanLocality(r rowScanner) (model.Locality, error) {
	var (
		l                   model.Locality
		id                  int64
		ward, notes, sn, su sql.NullString
		lat, lng, dupConf   sql.NullFloat64
		lastVer             sql.NullTime
		verified, healthy   bit
		checked             sql.NullTime
		cit, urg, sig       sql.NullString
		score               sql.NullInt64
		dupOf               sql.NullInt64
	)
	if err := r.Scan(&id, &l.Name, &ward, &l.Status, &lat, &lng, &notes, &sn, &su, &lastVer, &verified, &healthy,
		&checked, &cit, &urg, &score, &sig, &dupOf, &dupConf); err != nil {
		return l, err
	}
	l.ID = &id
	l.Ward, l.Notes, l.SourceName, l.SourceURL = ns(ward), ns(notes), ns(sn), ns(su)
	l.CitizenDescription, l.UrgencySignals = ns(cit), ns(sig)
	if lat.Valid {
		l.Latitude = &lat.Float64
	}
	if lng.Valid {
		l.Longitude = &lng.Float64
	}
	if lastVer.Valid {
		l.LastVerified = &model.Date{Time: lastVer.Time}
	}
	l.Verified = verified.V
	if healthy.Valid {
		l.SourceURLHealthy = &healthy.V
	}
	if checked.Valid {
		t := checked.Time.UTC()
		l.SourceURLLastChecked = &t
	}
	if urg.Valid {
		u := model.ReportUrgency(urg.String)
		l.ReportedUrgency = &u
	}
	if score.Valid {
		n := int(score.Int64)
		l.UrgencyScore = &n
	}
	if dupOf.Valid {
		l.PossibleDuplicateOfID = &dupOf.Int64
	}
	if dupConf.Valid {
		l.DuplicateConfidence = &dupConf.Float64
	}
	return l, nil
}

func ns(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func (s *SQL) ListLocalities(ctx context.Context) ([]model.Locality, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+locCols+" FROM locality ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Locality
	for rows.Next() {
		l, err := scanLocality(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *SQL) GetLocality(ctx context.Context, id int64) (model.Locality, error) {
	l, err := scanLocality(s.DB.QueryRowContext(ctx, "SELECT "+locCols+" FROM locality WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func locArgs(l model.Locality) []any {
	var lv any
	if l.LastVerified != nil {
		lv = l.LastVerified.Format("2006-01-02")
	}
	var urg any
	if l.ReportedUrgency != nil {
		urg = string(*l.ReportedUrgency)
	}
	var checked any
	if l.SourceURLLastChecked != nil {
		checked = l.SourceURLLastChecked.UTC().Format("2006-01-02 15:04:05.000000")
	}
	return []any{l.Name, l.Ward, string(l.Status), l.Latitude, l.Longitude, l.Notes, l.SourceName, l.SourceURL, lv,
		l.Verified, l.SourceURLHealthy, checked, l.CitizenDescription, urg, l.UrgencyScore, l.UrgencySignals,
		l.PossibleDuplicateOfID, l.DuplicateConfidence}
}

func (s *SQL) SaveLocality(ctx context.Context, l model.Locality) (model.Locality, error) {
	args := locArgs(l)
	if l.ID == nil {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO locality (name, ward, status, latitude, longitude, notes, source_name,
 source_url, last_verified, verified, source_url_healthy, source_url_last_checked, citizen_description, reported_urgency,
 urgency_score, urgency_signals, possible_duplicate_of_id, duplicate_confidence) VALUES (`+strings.TrimSuffix(strings.Repeat("?,", 18), ",")+`)`, args...)
		if err != nil {
			return l, err
		}
		id, err := res.LastInsertId()
		l.ID = &id
		return l, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE locality SET name=?, ward=?, status=?, latitude=?, longitude=?, notes=?, source_name=?,
 source_url=?, last_verified=?, verified=?, source_url_healthy=?, source_url_last_checked=?, citizen_description=?, reported_urgency=?,
 urgency_score=?, urgency_signals=?, possible_duplicate_of_id=?, duplicate_confidence=? WHERE id=?`, append(args, *l.ID)...)
	if err != nil {
		return l, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := s.GetLocality(ctx, *l.ID); gerr != nil { // MySQL reports 0 for "no change"
			return l, ErrNotFound
		}
	}
	return l, nil
}

func (s *SQL) DeleteLocality(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM locality WHERE id = ?", id)
	return err
}

func (s *SQL) ExistsLocalityByName(ctx context.Context, name string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM locality WHERE name = ?", name).Scan(&n)
	return n > 0, err
}

func (s *SQL) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_user").Scan(&n)
	return n, err
}

func (s *SQL) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, username, password_hash, role FROM app_user ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *SQL) FindUserByUsername(ctx context.Context, name string) (model.User, error) {
	var u model.User
	err := s.DB.QueryRowContext(ctx, "SELECT id, username, password_hash, role FROM app_user WHERE username = ?", name).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *SQL) CreateUser(ctx context.Context, u model.User) (model.User, error) {
	res, err := s.DB.ExecContext(ctx, "INSERT INTO app_user (username, password_hash, role) VALUES (?, ?, ?)", u.Username, u.PasswordHash, string(u.Role))
	if err != nil {
		if strings.Contains(err.Error(), "1062") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return u, ErrDuplicate
		}
		return u, err
	}
	u.ID, err = res.LastInsertId()
	return u, err
}

func (s *SQL) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM app_user WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQL) AddAudit(ctx context.Context, a model.AuditLog) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO audit_log (username, action, detail, success, error_message, `timestamp`) VALUES (?, ?, ?, ?, ?, ?)",
		a.Username, a.Action, a.Detail, a.Success, a.ErrorMessage, a.Timestamp.UTC().Format("2006-01-02 15:04:05.000000"))
	return err
}

func (s *SQL) ListAudit(ctx context.Context) ([]model.AuditLog, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, username, action, detail, success, error_message, `timestamp` FROM audit_log ORDER BY `timestamp` DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AuditLog
	for rows.Next() {
		var a model.AuditLog
		var detail, errMsg sql.NullString
		var ok bit
		var ts time.Time
		if err := rows.Scan(&a.ID, &a.Username, &a.Action, &detail, &ok, &errMsg, &ts); err != nil {
			return nil, err
		}
		a.Detail, a.ErrorMessage, a.Success, a.Timestamp = ns(detail), ns(errMsg), ok.V, ts.UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}
