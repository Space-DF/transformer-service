package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Space-DF/transformer-service/internal/models"
)

// LoggerService handles raw data logging for training purposes
type LoggerService struct {
	logDir          string
	enableFileLog   bool
	enableJSONLog   bool
	maxFileSize     int64 // in bytes
	currentLogFile  *os.File
	currentFileSize int64
}

// LoggerConfig contains configuration for the logger service
type LoggerConfig struct {
	LogDir        string `mapstructure:"log_dir" env:"RAW_DATA_LOG_DIR"`
	EnableFileLog bool   `mapstructure:"enable_file_log" env:"RAW_DATA_ENABLE_FILE_LOG"`
	EnableJSONLog bool   `mapstructure:"enable_json_log" env:"RAW_DATA_ENABLE_JSON_LOG"`
	MaxFileSize   int64  `mapstructure:"max_file_size" env:"RAW_DATA_MAX_FILE_SIZE"`
}

// NewLoggerService creates a new logger service
func NewLoggerService(config LoggerConfig) (*LoggerService, error) {
	ls := &LoggerService{
		logDir:        config.LogDir,
		enableFileLog: config.EnableFileLog,
		enableJSONLog: config.EnableJSONLog,
		maxFileSize:   config.MaxFileSize,
	}

	// Set defaults if not configured
	if ls.logDir == "" {
		ls.logDir = "logs/raw_data"
	}
	if ls.maxFileSize <= 0 {
		ls.maxFileSize = 100 * 1024 * 1024 // 100MB default
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
	decodedRawData interface{},
	processingInfo models.ProcessingInfo,
) error {

	// Extract device information
	deviceEUI := extractDeviceEUI(decodedRawData)
	deviceID := extractStringFromPayload(originalPayload, "device_id")
	deviceName := extractStringFromPayload(originalPayload, "device_name")
	eventType := extractStringFromPayload(originalPayload, "event_type")
	rawData := extractStringFromPayload(originalPayload, "raw_data")

	// Create log entry
	logEntry := models.RawDataLog{
		ID:              generateID(),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		DeviceEUI:       deviceEUI,
		DeviceID:        deviceID,
		DeviceName:      deviceName,
		EventType:       eventType,
		RawData:         rawData,
		DecodedRawData:  decodedRawData,
		OriginalPayload: originalPayload,
		ProcessingInfo:  processingInfo,
	}

	// Log to JSON format if enabled
	if ls.enableJSONLog {
		if err := ls.logToJSON(logEntry); err != nil {
			return fmt.Errorf("failed to log to JSON: %w", err)
		}
	}

	// Log to file if enabled
	if ls.enableFileLog {
		if err := ls.logToFile(logEntry); err != nil {
			return fmt.Errorf("failed to log to file: %w", err)
		}
	}

	return nil
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
	// Check if we need to rotate the log file
	if ls.currentLogFile == nil || ls.currentFileSize >= ls.maxFileSize {
		if err := ls.rotateLogFile(); err != nil {
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

	n, err := ls.currentLogFile.Write(jsonData)
	if err != nil {
		return err
	}

	ls.currentFileSize += int64(n)

	// Sync to ensure data is written to disk
	if err := ls.currentLogFile.Sync(); err != nil {
		return err
	}

	return nil
}

// rotateLogFile creates a new log file for rotation
func (ls *LoggerService) rotateLogFile() error {
	// Close current file if exists
	if ls.currentLogFile != nil {
		_ = ls.currentLogFile.Close()
	}

	// Create new log file with timestamp
	timestamp := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("raw_data_%s.jsonl", timestamp)
	filePath := filepath.Join(ls.logDir, filename)
	
	// Clean the file path to prevent path traversal attacks
	cleanPath := filepath.Clean(filePath)

	file, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	ls.currentLogFile = file
	ls.currentFileSize = 0

	return nil
}

// extractDeviceEUI extracts device EUI from various possible locations
func extractDeviceEUI(decodedData interface{}) string {
	if decodedData == nil {
		return ""
	}

	if dataMap, ok := decodedData.(map[string]interface{}); ok {
		// Try deviceInfo.devEui
		if deviceInfo, exists := dataMap["deviceInfo"].(map[string]interface{}); exists {
			if devEui, exists := deviceInfo["devEui"].(string); exists {
				return devEui
			}
		}

		// Try direct devEui field
		if devEui, exists := dataMap["devEui"].(string); exists {
			return devEui
		}

		// Try dev_eui field
		if devEui, exists := dataMap["dev_eui"].(string); exists {
			return devEui
		}
	}

	return ""
}

// extractStringFromPayload extracts a string value from the payload
func extractStringFromPayload(payload map[string]interface{}, key string) string {
	if value, exists := payload[key]; exists {
		if strVal, ok := value.(string); ok {
			return strVal
		}
	}
	return ""
}

// generateID generates a simple random ID
func generateID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Close closes the logger service and any open files
func (ls *LoggerService) Close() error {
	if ls.currentLogFile != nil {
		return ls.currentLogFile.Close()
	}
	return nil
}
