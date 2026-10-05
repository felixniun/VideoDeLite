// Package history persists task history in local SQLite.
// History never leaves this machine (boundary spec §19).
package history

import (
	"context"
	"database/sql"
	"time"
)

type Entry struct {
	ID             int64   `json:"id"`
	CreatedAt      string  `json:"createdAt"`
	InputName      string  `json:"inputName"`
	InputPath      string  `json:"inputPath"`
	InputSize      int64   `json:"inputSize"`
	OutputName     string  `json:"outputName"`
	OutputPath     string  `json:"outputPath"`
	OutputSize     int64   `json:"outputSize"`
	Container      string  `json:"container"`
	VideoCodec     string  `json:"videoCodec"`
	Encoder        string  `json:"encoder"`
	Quality        string  `json:"quality"`
	BitrateKbps    int64   `json:"bitrateKbps"`
	DurationSec    float64 `json:"durationSec"`
	EncodeTimeSec  float64 `json:"encodeTimeSec"`
	Ratio          float64 `json:"ratio"` // outputSize / inputSize
	Status         string  `json:"status"`
	ErrorSummary   string  `json:"errorSummary"`
	Warnings       string  `json:"warnings"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Add(e Entry) error {
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	_, err := s.db.ExecContext(context.Background(), `
		INSERT INTO history (
			created_at, input_name, input_path, input_size,
			output_name, output_path, output_size,
			container, video_codec, encoder, quality, bitrate_kbps,
			duration_sec, encode_time_sec, ratio,
			status, error_summary, warnings
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.CreatedAt, e.InputName, e.InputPath, e.InputSize,
		e.OutputName, e.OutputPath, e.OutputSize,
		e.Container, e.VideoCodec, e.Encoder, e.Quality, e.BitrateKbps,
		e.DurationSec, e.EncodeTimeSec, e.Ratio,
		e.Status, e.ErrorSummary, e.Warnings)
	return err
}

// List returns the most recent entries.
func (s *Store) List(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(context.Background(), `
		SELECT id, created_at, input_name, input_path, input_size,
		       output_name, output_path, output_size,
		       container, video_codec, encoder, quality, bitrate_kbps,
		       duration_sec, encode_time_sec, ratio,
		       status, error_summary, warnings
		FROM history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.InputName, &e.InputPath, &e.InputSize,
			&e.OutputName, &e.OutputPath, &e.OutputSize,
			&e.Container, &e.VideoCodec, &e.Encoder, &e.Quality, &e.BitrateKbps,
			&e.DurationSec, &e.EncodeTimeSec, &e.Ratio,
			&e.Status, &e.ErrorSummary, &e.Warnings); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Clear removes history records only — never videos (plan §40).
func (s *Store) Clear() error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM history`)
	return err
}
