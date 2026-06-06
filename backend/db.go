package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Session 会话
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message 消息
type Message struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// DB 数据库操作封装
type DB struct {
	conn *sql.DB
}

// InitDB 初始化数据库
func InitDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	conn.SetMaxOpenConns(1)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	return d, nil
}

func (d *DB) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '新对话',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system')),
		content TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
	`
	_, err := d.conn.Exec(query)
	return err
}


func genID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession 创建新会话
func (d *DB) CreateSession(userID, title string) (*Session, error) {
	id, err := genID()
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = "新对话"
	}
	now := time.Now()
	_, err = d.conn.Exec(
		"INSERT INTO sessions (id, user_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		id, userID, title, now, now,
	)
	if err != nil {
		return nil, err
	}
	return &Session{ID: id, UserID: userID, Title: title, CreatedAt: now, UpdatedAt: now}, nil
}

// ListSessions 列出指定用户的会话（按更新时间倒序）
func (d *DB) ListSessions(userID string) ([]Session, error) {
	rows, err := d.conn.Query(
		"SELECT id, user_id, title, created_at, updated_at FROM sessions WHERE user_id = ? ORDER BY updated_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetSession 获取单个会话
func (d *DB) GetSession(id string) (*Session, []Message, error) {
	var s Session
	err := d.conn.QueryRow("SELECT id, user_id, title, created_at, updated_at FROM sessions WHERE id = ?", id).
		Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	messages, err := d.GetMessages(id)
	if err != nil {
		return nil, nil, err
	}
	return &s, messages, nil
}

// UpdateSession 更新会话标题
func (d *DB) UpdateSession(id, title string) error {
	_, err := d.conn.Exec(
		"UPDATE sessions SET title = ?, updated_at = datetime('now') WHERE id = ?",
		title, id,
	)
	return err
}

// DeleteSession 删除会话
func (d *DB) DeleteSession(id string) error {
	_, err := d.conn.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// GetMessages 获取会话的所有消息（按时间顺序）
func (d *DB) GetMessages(sessionID string) ([]Message, error) {
	rows, err := d.conn.Query(
		"SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = ? ORDER BY created_at ASC",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// AddMessage 添加消息
func (d *DB) AddMessage(sessionID, role, content string) (*Message, error) {
	result, err := d.conn.Exec(
		"INSERT INTO messages (session_id, role, content) VALUES (?, ?, ?)",
		sessionID, role, content,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()

	d.conn.Exec("UPDATE sessions SET updated_at = datetime('now') WHERE id = ?", sessionID)

	if role == "user" {
		d.conn.Exec(
			"UPDATE sessions SET title = ? WHERE id = ? AND title = '新对话'",
			truncateTitle(content, 30), sessionID,
		)
	}

	return &Message{
		ID:        id,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}, nil
}

func truncateTitle(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}

// UpdateMessageContent 更新指定消息的内容
func (d *DB) UpdateMessageContent(id int64, content string) error {
	_, err := d.conn.Exec("UPDATE messages SET content = ? WHERE id = ?", content, id)
	return err
}

// DeleteMessagesAfter 删除指定消息之后的所有消息（按 id 递增顺序）
func (d *DB) DeleteMessagesAfter(sessionID string, afterID int64) error {
	_, err := d.conn.Exec("DELETE FROM messages WHERE session_id = ? AND id > ?", sessionID, afterID)
	return err
}

// DeleteMessagesFrom 删除指定消息及之后的所有消息（用于重新生成场景）
func (d *DB) DeleteMessagesFrom(sessionID string, fromID int64) error {
	_, err := d.conn.Exec("DELETE FROM messages WHERE session_id = ? AND id >= ?", sessionID, fromID)
	return err
}

// GetMessageByID 获取单个消息
func (d *DB) GetMessageByID(id int64) (*Message, error) {
	var m Message
	err := d.conn.QueryRow(
		"SELECT id, session_id, role, content, created_at FROM messages WHERE id = ?", id,
	).Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Close 关闭数据库连接
func (d *DB) Close() error {
	return d.conn.Close()
}
