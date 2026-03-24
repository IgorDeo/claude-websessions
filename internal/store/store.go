package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type SessionRecord struct {
	ID        string
	Name      string
	ClaudeID  string
	WorkDir   string
	StartTime time.Time
	EndTime   time.Time
	ExitCode  int
	Status    string
	PID       int
}

type NotificationRecord struct {
	ID        int64
	SessionID string
	EventType string
	Timestamp time.Time
	Read      bool
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY, name TEXT, claude_id TEXT, work_dir TEXT,
		start_time DATETIME, end_time DATETIME,
		exit_code INTEGER, status TEXT, pid INTEGER
	);
	CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT, event_type TEXT,
		timestamp DATETIME, read BOOLEAN DEFAULT FALSE
	);
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action TEXT, session_id TEXT, client_ip TEXT, timestamp DATETIME
	);`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	// Migration: add name column if it doesn't exist (for existing DBs)
	db.Exec("ALTER TABLE sessions ADD COLUMN name TEXT DEFAULT ''")
	return nil
}

func (s *Store) SaveSession(r SessionRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, name, claude_id, work_dir, start_time, end_time, exit_code, status, pid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.ClaudeID, r.WorkDir, r.StartTime, r.EndTime, r.ExitCode, r.Status, r.PID,
	)
	return err
}

func (s *Store) ListSessions(limit int) ([]SessionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(name, ''), claude_id, work_dir, start_time, end_time, exit_code, status, pid FROM sessions ORDER BY start_time DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.ClaudeID, &r.WorkDir, &r.StartTime, &r.EndTime, &r.ExitCode, &r.Status, &r.PID); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) SaveNotification(n NotificationRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO notifications (session_id, event_type, timestamp, read) VALUES (?, ?, ?, ?)`,
		n.SessionID, n.EventType, n.Timestamp, n.Read,
	)
	return err
}

func (s *Store) ListNotifications(limit int, includeRead bool) ([]NotificationRecord, error) {
	query := `SELECT id, session_id, event_type, timestamp, read FROM notifications`
	if !includeRead {
		query += ` WHERE read = FALSE`
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []NotificationRecord
	for rows.Next() {
		var r NotificationRecord
		if err := rows.Scan(&r.ID, &r.SessionID, &r.EventType, &r.Timestamp, &r.Read); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) MarkNotificationRead(id int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET read = TRUE WHERE id = ?`, id)
	return err
}

func (s *Store) MarkAllNotificationsRead() error {
	_, err := s.db.Exec(`UPDATE notifications SET read = TRUE WHERE read = FALSE`)
	return err
}

// RecentDirs returns distinct working directories from recent sessions, most recent first.
func (s *Store) RecentDirs(limit int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT work_dir FROM sessions WHERE work_dir != '' ORDER BY start_time DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dirs []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		dirs = append(dirs, d)
	}
	return dirs, rows.Err()
}

func (s *Store) LogAudit(action, sessionID, clientIP string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (action, session_id, client_ip, timestamp) VALUES (?, ?, ?, ?)`,
		action, sessionID, clientIP, time.Now(),
	)
	return err
}
