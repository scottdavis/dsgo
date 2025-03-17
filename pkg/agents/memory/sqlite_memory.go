package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/scottdavis/dsgo/pkg/agents"
	"github.com/scottdavis/dsgo/pkg/errors"
)

// SQLiteStore implements the Memory interface using SQLite as the backend.
type SQLiteStore struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string

	initialized sync.Once
}

// Ensure SQLiteStore implements Memory interface
var _ Memory = (*SQLiteStore)(nil)

// NewSQLiteStore creates a new SQLite-backed memory store.
// The path parameter specifies the database file location.
// If path is ":memory:", the database will be created in-memory.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	var connStr string
	if path == ":memory:" {
		connStr = path + "?cache=shared&mode=memory"
	} else {
		connStr = path + "?cache=shared"
	}

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to open SQLite database"),
			errors.Fields{"path": path},
		)
	}

	store := &SQLiteStore{
		db:   db,
		path: path,
	}
	if err := store.ensureInitialized(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) ensureInitialized() error {
	var initErr error
	s.initialized.Do(func() {
		// Enable WAL mode for better concurrency
		if _, err := s.db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			initErr = errors.WithFields(
				errors.Wrap(err, errors.Unknown, "failed to enable WAL mode"),
				errors.Fields{},
			)
			return
		}

		// Create table with JSON value column and metadata
		query := `
        CREATE TABLE IF NOT EXISTS memory_store (
            key TEXT PRIMARY KEY,
            value TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            expires_at DATETIME
        );
        
        -- Create index on created_at for efficient querying
        CREATE INDEX IF NOT EXISTS idx_memory_store_created_at 
        ON memory_store(created_at);
        `

		if _, err := s.db.Exec(query); err != nil {
			initErr = errors.WithFields(
				errors.Wrap(err, errors.Unknown, "failed to initialize database"),
				errors.Fields{"query": query},
			)
			return
		}
	})
	return initErr
}

// Store implements the Memory interface Store method.
func (s *SQLiteStore) Store(key string, value any, opts ...agents.StoreOption) error {
	if err := s.ensureInitialized(); err != nil {
		return err
	}

	// Process options
	options := &agents.StoreOptions{}
	for _, opt := range opts {
		opt(options)
	}

	jsonValue, err := json.Marshal(value)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.InvalidInput, "failed to marshal value to JSON"),
			errors.Fields{
				"key":        key,
				"value_type": fmt.Sprintf("%T", value),
			},
		)
	}

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to begin transaction")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Determine expiry time if TTL is set
	var expiryTime *time.Time
	if options.TTL > 0 {
		t := time.Now().Add(options.TTL)
		expiryTime = &t
	}

	// Delete existing key if it exists
	_, err = tx.Exec("DELETE FROM memory_store WHERE key = ?", key)
	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to delete existing key"),
			errors.Fields{"key": key},
		)
	}

	// Insert new key with JSON value
	var stmt *sql.Stmt
	if expiryTime != nil {
		stmt, err = tx.Prepare("INSERT INTO memory_store (key, value, updated_at, expires_at) VALUES (?, ?, ?, ?)")
		if err != nil {
			return errors.Wrap(err, errors.Unknown, "failed to prepare insert statement")
		}
		_, err = stmt.Exec(key, string(jsonValue), time.Now().Format(time.RFC3339), expiryTime.Format(time.RFC3339))
	} else {
		stmt, err = tx.Prepare("INSERT INTO memory_store (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)")
		if err != nil {
			return errors.Wrap(err, errors.Unknown, "failed to prepare insert statement")
		}
		_, err = stmt.Exec(key, string(jsonValue))
	}

	if err != nil {
		return errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to store value in SQLite"),
			errors.Fields{"key": key},
		)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to commit transaction")
	}

	return nil
}

// Retrieve implements the Memory interface Retrieve method.
func (s *SQLiteStore) Retrieve(key string) (any, error) {
	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jsonValue string
	query := `SELECT value FROM memory_store WHERE key = ?`

	err := s.db.QueryRow(query, key).Scan(&jsonValue)
	if err == sql.ErrNoRows {
		return nil, errors.WithFields(
			errors.New(errors.ResourceNotFound, "key not found"),
			errors.Fields{"key": key},
		)
	}
	if err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.Unknown, "failed to retrieve value"),
			errors.Fields{"key": key},
		)
	}

	var value any
	if err := json.Unmarshal([]byte(jsonValue), &value); err != nil {
		return nil, errors.WithFields(
			errors.Wrap(err, errors.InvalidResponse, "failed to unmarshal value from JSON"),
			errors.Fields{"key": key, "json_value": jsonValue},
		)
	}

	switch v := value.(type) {
	case map[string]any:
		// Check if all values are numbers for map[string]int
		allInts := true
		for _, val := range v {
			if _, ok := val.(float64); !ok {
				allInts = false
				break
			}
		}
		if allInts {
			intMap := make(map[string]int)
			for key, val := range v {
				intMap[key] = int(val.(float64))
			}
			return intMap, nil
		}
	case []any:
		// Check if it's a string array
		allStrings := true
		for _, item := range v {
			if _, ok := item.(string); !ok {
				allStrings = false
				break
			}
		}
		if allStrings {
			strArr := make([]string, len(v))
			for i, item := range v {
				strArr[i] = item.(string)
			}
			return strArr, nil
		}
	case float64:
		// Convert to int if it's a whole number
		if v == float64(int(v)) {
			return int(v), nil
		}
	}

	return value, nil
}

// List implements the Memory interface List method.
func (s *SQLiteStore) List() ([]string, error) {
	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT key FROM memory_store ORDER BY created_at")
	if err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "failed to list keys")
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, errors.Wrap(err, errors.Unknown, "failed to scan key")
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.Unknown, "error iterating rows")
	}

	return keys, nil
}

// Clear implements the Memory interface Clear method.
func (s *SQLiteStore) Clear() error {
	if err := s.ensureInitialized(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM memory_store")
	if err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to clear memory store")
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.db.Close(); err != nil {
		return errors.Wrap(err, errors.Unknown, "failed to close database connection")
	}
	return nil
}

// CleanExpired removes all expired entries from the store.
func (s *SQLiteStore) CleanExpired(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
    DELETE FROM memory_store 
    WHERE expires_at IS NOT NULL 
    AND datetime(expires_at) <= datetime('now', 'utc')`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, errors.Wrap(err, errors.Unknown, "failed to clean expired entries")
	}

	// We don't need to return an error for successful cleanup
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, errors.Unknown, "failed to get affected rows count")
	}

	return affected, nil
}
