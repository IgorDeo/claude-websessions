package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type SessionRecord struct {
	ID          string
	Name        string
	ClaudeID    string
	WorkDir     string
	StartTime   time.Time
	EndTime     time.Time
	ExitCode    int
	Status      string
	PID         int
	Sandboxed   bool
	SandboxName string
	TeamName    string
	TeamRole    string
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
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	if err := migrate(db); err != nil {
		_ = db.Close()
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
	);
	CREATE TABLE IF NOT EXISTS preferences (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	CREATE TABLE IF NOT EXISTS session_output (
		session_id TEXT PRIMARY KEY,
		data BLOB,
		updated_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS metrics_samples (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		metric    TEXT NOT NULL,
		value     REAL NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_metric_ts ON metrics_samples(metric, timestamp);`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	// Migration: add name column if it doesn't exist (for existing DBs)
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN name TEXT DEFAULT ''")
	// Migration: add sandbox columns
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN sandboxed BOOLEAN DEFAULT FALSE")
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN sandbox_name TEXT DEFAULT ''")
	// Migration: add team columns
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN team_name TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE sessions ADD COLUMN team_role TEXT DEFAULT ''")

	// Agent Teams tables
	teamsSchema := `
	CREATE TABLE IF NOT EXISTS teams (
		name TEXT PRIMARY KEY,
		state TEXT,
		lead_session_id TEXT,
		discovered_at DATETIME,
		last_seen DATETIME
	);
	CREATE TABLE IF NOT EXISTS team_tasks (
		id TEXT PRIMARY KEY,
		team_name TEXT,
		title TEXT,
		description TEXT,
		state TEXT,
		assigned_to TEXT,
		created_at DATETIME,
		updated_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS team_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_name TEXT,
		from_member TEXT,
		to_member TEXT,
		content TEXT,
		timestamp DATETIME
	);`
	_, err = db.Exec(teamsSchema)
	return err
}

func (s *Store) SaveSession(r SessionRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, name, claude_id, work_dir, start_time, end_time, exit_code, status, pid, sandboxed, sandbox_name, team_name, team_role) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.ClaudeID, r.WorkDir, r.StartTime, r.EndTime, r.ExitCode, r.Status, r.PID, r.Sandboxed, r.SandboxName, r.TeamName, r.TeamRole,
	)
	return err
}

func (s *Store) ListSessions(limit int) ([]SessionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(name, ''), claude_id, work_dir, start_time, end_time, exit_code, status, pid, COALESCE(sandboxed, 0), COALESCE(sandbox_name, ''), COALESCE(team_name, ''), COALESCE(team_role, '') FROM sessions ORDER BY start_time DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.ClaudeID, &r.WorkDir, &r.StartTime, &r.EndTime, &r.ExitCode, &r.Status, &r.PID, &r.Sandboxed, &r.SandboxName, &r.TeamName, &r.TeamRole); err != nil {
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
	defer rows.Close() //nolint:errcheck
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

func (s *Store) GetSession(id string) (*SessionRecord, error) {
	var r SessionRecord
	err := s.db.QueryRow(
		`SELECT id, COALESCE(name, ''), claude_id, work_dir, start_time, end_time, exit_code, status, pid, COALESCE(sandboxed, 0), COALESCE(sandbox_name, ''), COALESCE(team_name, ''), COALESCE(team_role, '') FROM sessions WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.ClaudeID, &r.WorkDir, &r.StartTime, &r.EndTime, &r.ExitCode, &r.Status, &r.PID, &r.Sandboxed, &r.SandboxName, &r.TeamName, &r.TeamRole)
	if err != nil {
		return nil, err
	}
	return &r, nil
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
	defer rows.Close() //nolint:errcheck
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

func (s *Store) GetPreference(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM preferences WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetPreference(key, value string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO preferences (key, value) VALUES (?, ?)`,
		key, value,
	)
	return err
}

func (s *Store) GetAllPreferences() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM preferences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	prefs := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		prefs[k] = v
	}
	return prefs, rows.Err()
}

func (s *Store) SaveOutput(sessionID string, data []byte) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO session_output (session_id, data, updated_at) VALUES (?, ?, ?)`,
		sessionID, data, time.Now(),
	)
	return err
}

func (s *Store) LoadOutput(sessionID string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM session_output WHERE session_id = ?`, sessionID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Store) DeleteOutput(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM session_output WHERE session_id = ?`, sessionID)
	return err
}

func (s *Store) LogAudit(action, sessionID, clientIP string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (action, session_id, client_ip, timestamp) VALUES (?, ?, ?, ?)`,
		action, sessionID, clientIP, time.Now(),
	)
	return err
}

// MetricSample represents a single time-series data point.
type MetricSample struct {
	Timestamp time.Time
	Value     float64
}

// SaveMetricSamples writes a batch of metric samples in a single transaction.
func (s *Store) SaveMetricSamples(samples map[string]float64, ts time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO metrics_samples (timestamp, metric, value) VALUES (?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close() //nolint:errcheck
	for metric, value := range samples {
		if _, err := stmt.Exec(ts, metric, value); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// QueryMetrics returns time-series data for a single metric within a time range.
func (s *Store) QueryMetrics(metric string, from, to time.Time) ([]MetricSample, error) {
	rows, err := s.db.Query(
		`SELECT timestamp, value FROM metrics_samples WHERE metric = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC`,
		metric, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var samples []MetricSample
	for rows.Next() {
		var m MetricSample
		if err := rows.Scan(&m.Timestamp, &m.Value); err != nil {
			return nil, err
		}
		samples = append(samples, m)
	}
	return samples, rows.Err()
}

// QueryAllMetrics returns time-series data for all metrics within a time range.
func (s *Store) QueryAllMetrics(from, to time.Time) (map[string][]MetricSample, error) {
	rows, err := s.db.Query(
		`SELECT metric, timestamp, value FROM metrics_samples WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	result := make(map[string][]MetricSample)
	for rows.Next() {
		var metric string
		var m MetricSample
		if err := rows.Scan(&metric, &m.Timestamp, &m.Value); err != nil {
			return nil, err
		}
		result[metric] = append(result[metric], m)
	}
	return result, rows.Err()
}

// PruneMetrics deletes metric samples older than the given time.
func (s *Store) PruneMetrics(before time.Time) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM metrics_samples WHERE timestamp < ?`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Agent Teams ---

type TeamRecord struct {
	Name          string
	State         string
	LeadSessionID string
	DiscoveredAt  time.Time
	LastSeen      time.Time
}

type TeamTaskRecord struct {
	ID         string
	TeamName   string
	Title      string
	Description string
	State      string
	AssignedTo string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type TeamMessageRecord struct {
	ID         int64
	TeamName   string
	FromMember string
	ToMember   string
	Content    string
	Timestamp  time.Time
}

func (s *Store) SaveTeam(r TeamRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO teams (name, state, lead_session_id, discovered_at, last_seen) VALUES (?, ?, ?, ?, ?)`,
		r.Name, r.State, r.LeadSessionID, r.DiscoveredAt, r.LastSeen,
	)
	return err
}

func (s *Store) ListTeams() ([]TeamRecord, error) {
	rows, err := s.db.Query(`SELECT name, state, lead_session_id, discovered_at, last_seen FROM teams ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var records []TeamRecord
	for rows.Next() {
		var r TeamRecord
		if err := rows.Scan(&r.Name, &r.State, &r.LeadSessionID, &r.DiscoveredAt, &r.LastSeen); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) SaveTeamTask(r TeamTaskRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO team_tasks (id, team_name, title, description, state, assigned_to, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TeamName, r.Title, r.Description, r.State, r.AssignedTo, r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *Store) ListTeamTasks(teamName string) ([]TeamTaskRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, team_name, title, description, state, assigned_to, created_at, updated_at FROM team_tasks WHERE team_name = ? ORDER BY created_at`,
		teamName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var records []TeamTaskRecord
	for rows.Next() {
		var r TeamTaskRecord
		if err := rows.Scan(&r.ID, &r.TeamName, &r.Title, &r.Description, &r.State, &r.AssignedTo, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) SaveTeamMessage(r TeamMessageRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO team_messages (team_name, from_member, to_member, content, timestamp) VALUES (?, ?, ?, ?, ?)`,
		r.TeamName, r.FromMember, r.ToMember, r.Content, r.Timestamp,
	)
	return err
}

func (s *Store) ListTeamMessages(teamName string, limit int) ([]TeamMessageRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, team_name, from_member, to_member, content, timestamp FROM team_messages WHERE team_name = ? ORDER BY timestamp DESC LIMIT ?`,
		teamName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var records []TeamMessageRecord
	for rows.Next() {
		var r TeamMessageRecord
		if err := rows.Scan(&r.ID, &r.TeamName, &r.FromMember, &r.ToMember, &r.Content, &r.Timestamp); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
