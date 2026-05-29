package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
	"github.com/google/uuid"
)

// LoggerService handles raw data logging for training purposes
type LoggerService struct {
	logDir          string
	enableFileLog   bool
	enableJSONLog   bool
	maxFileSize     int64 // in bytes
	currentLogFile  *os.File
	currentFileSize int64
	mu              sync.Mutex
}

// LoggerConfig contains configuration for the logger service
type LoggerConfig struct {
	LogDir        string `mapstructure:"log_dir" env:"RAW_DATA_LOG_DIR"`
	EnableFileLog bool   `mapstructure:"enable_file_log" env:"RAW_DATA_ENABLE_FILE_LOG"`
	EnableJSONLog bool   `mapstructure:"enable_json_log" env:"RAW_DATA_ENABLE_JSON_LOG"`
}

// NewLoggerService creates a new logger service
func NewLoggerService(config LoggerConfig) (*LoggerService, error) {
	ls := &LoggerService{
		logDir:        config.LogDir,
		enableFileLog: config.EnableFileLog,
		enableJSONLog: config.EnableJSONLog,
	}

	// Set defaults if not configured
	if ls.logDir == "" {
		ls.logDir = "logs/raw_data"
	}

	// Create log directory if file logging is enabled
	if ls.enableFileLog {
		if err := os.MkdirAll(ls.logDir, 0750); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	return ls, nil
}

// LogRawData logs raw data for training purposes
func (ls *LoggerService) LogRawData(
	originalPayload map[string]interface{},
	devEUI string,
	processingInfo models.ProcessingInfo,
) (models.RawDataLog, error) {

	// Create log entry
	logEntry := models.RawDataLog{
		ID:              uuid.New().String(),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		DeviceEUI:       devEUI,
		OriginalPayload: originalPayload,
		ProcessingInfo:  processingInfo,
	}

	// Log to JSON format if enabled
	if ls.enableJSONLog {
		if err := ls.logToJSON(logEntry); err != nil {
			return logEntry, fmt.Errorf("failed to log to JSON: %w", err)
		}
	}

	// Log to file if enabled
	if ls.enableFileLog {
		if err := ls.logToFile(logEntry); err != nil {
			return logEntry, fmt.Errorf("failed to log to file: %w", err)
		}
	}

	return logEntry, nil
}

// logToJSON logs the entry as structured JSON to stdout
func (ls *LoggerService) logToJSON(entry models.RawDataLog) error {
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	fmt.Printf("RAW_DATA_LOG: %s\n", string(jsonData))
	return nil
}

// logToFile logs the entry to a file
func (ls *LoggerService) logToFile(entry models.RawDataLog) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	// Check if we need to rotate the log file
	if ls.currentLogFile == nil || ls.currentFileSize >= ls.maxFileSize {
		if err := ls.openLogFile(); err != nil {
			return err
		}
	}

	// Convert to JSON and write to file
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Add newline for proper JSONL format
	jsonData = append(jsonData, '\n')

	_, err = ls.currentLogFile.Write(jsonData)
	if err != nil {
		return err
	}

	if err := ls.currentLogFile.Sync(); err != nil {
		return err
	}

	return nil
}

// openLogFile opens the single fixed log file
func (ls *LoggerService) openLogFile() error {
	// Close current file if exists
	if ls.currentLogFile != nil {
		_ = ls.currentLogFile.Close()
	}

	filename := "raw_data.jsonl"
	filePath := filepath.Join(ls.logDir, filename)

	// Clean the file path to prevent path traversal attacks
	cleanPath := filepath.Clean(filePath)

	file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	ls.currentLogFile = file

	return nil
}

// Close closes the logger service and any open files
func (ls *LoggerService) Close() error {
	if ls.currentLogFile != nil {
		return ls.currentLogFile.Close()
	}
	return nil
}
