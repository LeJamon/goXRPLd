package relational_db

import (
	"errors"
	"fmt"
	"strings"
)

// Common database errors
var (
	// Connection errors
	ErrDatabaseNotOpen     = errors.New("database is not open")
	ErrDatabaseAlreadyOpen = errors.New("database is already open")
	ErrConnectionFailed    = errors.New("failed to connect to database")
	ErrConnectionTimeout   = errors.New("database connection timeout")
	ErrConnectionLost      = errors.New("database connection lost")
	ErrTooManyConnections  = errors.New("too many database connections")

	// Transaction errors
	ErrTransactionNotFound   = errors.New("transaction not found")
	ErrTransactionInProgress = errors.New("transaction already in progress")
	ErrTransactionCommitted  = errors.New("transaction already committed")
	ErrTransactionRolledBack = errors.New("transaction already rolled back")
	ErrDeadlock              = errors.New("database deadlock detected")

	// Ledger errors
	ErrLedgerNotFound        = errors.New("ledger not found")
	ErrLedgerAlreadyExists   = errors.New("ledger already exists")
	ErrInvalidLedgerSequence = errors.New("invalid ledger sequence")
	ErrLedgerHashMismatch    = errors.New("ledger hash mismatch")

	// Account errors
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidAccountID  = errors.New("invalid account ID")
	ErrAccountTxNotFound = errors.New("account transaction not found")

	// Query errors
	ErrInvalidQuery      = errors.New("invalid query")
	ErrQueryTimeout      = errors.New("query timeout")
	ErrResultSetTooLarge = errors.New("result set too large")
	ErrInvalidOffset     = errors.New("invalid offset")
	ErrInvalidLimit      = errors.New("invalid limit")
	ErrInvalidMarker     = errors.New("invalid pagination marker")

	// Data errors
	ErrDataCorruption        = errors.New("data corruption detected")
	ErrInvalidData           = errors.New("invalid data format")
	ErrDataTooLarge          = errors.New("data too large")
	ErrSerializationFailed   = errors.New("serialization failed")
	ErrDeserializationFailed = errors.New("deserialization failed")

	// Configuration errors
	ErrInvalidConfig        = errors.New("invalid configuration")
	ErrConfigurationMissing = errors.New("configuration missing")
	ErrUnsupportedDriver    = errors.New("unsupported database driver")

	// Storage errors
	ErrDiskFull          = errors.New("disk full")
	ErrInsufficientSpace = errors.New("insufficient disk space")
	ErrFileCorruption    = errors.New("database file corruption")
	ErrPermissionDenied  = errors.New("permission denied")

	// Constraint errors
	ErrConstraintViolation  = errors.New("constraint violation")
	ErrUniqueConstraint     = errors.New("unique constraint violation")
	ErrForeignKeyConstraint = errors.New("foreign key constraint violation")
	ErrCheckConstraint      = errors.New("check constraint violation")
)

// DatabaseError represents a database-specific error with additional context
type DatabaseError struct {
	Code       string
	Message    string
	Operation  string
	Table      string
	Query      string
	Parameters []interface{}
	Cause      error
	Retryable  bool
	Temporary  bool
}

func (e *DatabaseError) Error() string {
	var parts []string

	if e.Code != "" {
		parts = append(parts, fmt.Sprintf("code=%s", e.Code))
	}

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation=%s", e.Operation))
	}

	if e.Table != "" {
		parts = append(parts, fmt.Sprintf("table=%s", e.Table))
	}

	if e.Message != "" {
		parts = append(parts, e.Message)
	}

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause=%v", e.Cause))
	}

	return strings.Join(parts, " | ")
}

func (e *DatabaseError) Unwrap() error {
	return e.Cause
}

func (e *DatabaseError) Is(target error) bool {
	if target == nil {
		return false
	}

	if e.Cause != nil && errors.Is(e.Cause, target) {
		return true
	}

	return e.Error() == target.Error()
}

// IsRetryable returns true if the error is retryable
func (e *DatabaseError) IsRetryable() bool {
	return e.Retryable
}

// IsTemporary returns true if the error is temporary
func (e *DatabaseError) IsTemporary() bool {
	return e.Temporary
}

// NewDatabaseError creates a new database error
func NewDatabaseError(code, message, operation string, cause error) *DatabaseError {
	return &DatabaseError{
		Code:      code,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: isRetryableError(cause),
		Temporary: isTemporaryError(cause),
	}
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field=%s, value=%v, message=%s",
		e.Field, e.Value, e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// ConfigError represents a configuration error
type ConfigError struct {
	Setting string
	Value   interface{}
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: setting=%s, value=%v, message=%s",
		e.Setting, e.Value, e.Message)
}

// NewConfigError creates a new configuration error
func NewConfigError(setting string, value interface{}, message string) *ConfigError {
	return &ConfigError{
		Setting: setting,
		Value:   value,
		Message: message,
	}
}

// Helper functions to determine error characteristics
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific retryable errors
	switch {
	case errors.Is(err, ErrConnectionTimeout):
		return true
	case errors.Is(err, ErrConnectionLost):
		return true
	case errors.Is(err, ErrQueryTimeout):
		return true
	case errors.Is(err, ErrDeadlock):
		return true
	case errors.Is(err, ErrTooManyConnections):
		return true
	default:
		return false
	}
}

func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific temporary errors
	switch {
	case errors.Is(err, ErrConnectionTimeout):
		return true
	case errors.Is(err, ErrConnectionLost):
		return true
	case errors.Is(err, ErrQueryTimeout):
		return true
	case errors.Is(err, ErrTooManyConnections):
		return true
	case errors.Is(err, ErrDiskFull):
		return false // Not temporary, needs intervention
	case errors.Is(err, ErrInsufficientSpace):
		return false // Not temporary, needs intervention
	default:
		return false
	}
}

// Error checking utility functions
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrConnectionFailed) ||
		errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrConnectionLost) ||
		errors.Is(err, ErrTooManyConnections)
}

func IsTransactionError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrTransactionNotFound) ||
		errors.Is(err, ErrTransactionInProgress) ||
		errors.Is(err, ErrTransactionCommitted) ||
		errors.Is(err, ErrTransactionRolledBack) ||
		errors.Is(err, ErrDeadlock)
}

func IsDataError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrDataCorruption) ||
		errors.Is(err, ErrInvalidData) ||
		errors.Is(err, ErrDataTooLarge) ||
		errors.Is(err, ErrSerializationFailed) ||
		errors.Is(err, ErrDeserializationFailed)
}

func IsStorageError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrDiskFull) ||
		errors.Is(err, ErrInsufficientSpace) ||
		errors.Is(err, ErrFileCorruption) ||
		errors.Is(err, ErrPermissionDenied)
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrTransactionNotFound) ||
		errors.Is(err, ErrLedgerNotFound) ||
		errors.Is(err, ErrAccountNotFound) ||
		errors.Is(err, ErrAccountTxNotFound)
}

func IsValidationError(err error) bool {
	if err == nil {
		return false
	}

	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

func IsConfigError(err error) bool {
	if err == nil {
		return false
	}

	var configErr *ConfigError
	return errors.As(err, &configErr)
}

func IsDatabaseError(err error) bool {
	if err == nil {
		return false
	}

	var dbErr *DatabaseError
	return errors.As(err, &dbErr)
}

// Error categorization for metrics and logging
type ErrorCategory int

const (
	ErrorCategoryUnknown ErrorCategory = iota
	ErrorCategoryConnection
	ErrorCategoryTransaction
	ErrorCategoryQuery
	ErrorCategoryData
	ErrorCategoryStorage
	ErrorCategoryValidation
	ErrorCategoryConfiguration
	ErrorCategoryConstraint
)

func (ec ErrorCategory) String() string {
	switch ec {
	case ErrorCategoryConnection:
		return "connection"
	case ErrorCategoryTransaction:
		return "transaction"
	case ErrorCategoryQuery:
		return "query"
	case ErrorCategoryData:
		return "data"
	case ErrorCategoryStorage:
		return "storage"
	case ErrorCategoryValidation:
		return "validation"
	case ErrorCategoryConfiguration:
		return "configuration"
	case ErrorCategoryConstraint:
		return "constraint"
	default:
		return "unknown"
	}
}

// CategorizeError categorizes an error for metrics and logging
func CategorizeError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	switch {
	case IsConnectionError(err):
		return ErrorCategoryConnection
	case IsTransactionError(err):
		return ErrorCategoryTransaction
	case IsDataError(err):
		return ErrorCategoryData
	case IsStorageError(err):
		return ErrorCategoryStorage
	case IsValidationError(err):
		return ErrorCategoryValidation
	case IsConfigError(err):
		return ErrorCategoryConfiguration
	case errors.Is(err, ErrConstraintViolation) ||
		errors.Is(err, ErrUniqueConstraint) ||
		errors.Is(err, ErrForeignKeyConstraint) ||
		errors.Is(err, ErrCheckConstraint):
		return ErrorCategoryConstraint
	case errors.Is(err, ErrInvalidQuery) ||
		errors.Is(err, ErrQueryTimeout) ||
		errors.Is(err, ErrResultSetTooLarge):
		return ErrorCategoryQuery
	default:
		return ErrorCategoryUnknown
	}
}

// ErrorSeverity represents the severity level of an error
type ErrorSeverity int

const (
	ErrorSeverityLow ErrorSeverity = iota
	ErrorSeverityMedium
	ErrorSeverityHigh
	ErrorSeverityCritical
)

func (es ErrorSeverity) String() string {
	switch es {
	case ErrorSeverityLow:
		return "low"
	case ErrorSeverityMedium:
		return "medium"
	case ErrorSeverityHigh:
		return "high"
	case ErrorSeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// GetErrorSeverity determines the severity of an error
func GetErrorSeverity(err error) ErrorSeverity {
	if err == nil {
		return ErrorSeverityLow
	}

	switch {
	case errors.Is(err, ErrDataCorruption) ||
		errors.Is(err, ErrFileCorruption):
		return ErrorSeverityCritical
	case errors.Is(err, ErrDiskFull) ||
		errors.Is(err, ErrInsufficientSpace) ||
		errors.Is(err, ErrConnectionFailed):
		return ErrorSeverityHigh
	case errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrQueryTimeout) ||
		errors.Is(err, ErrDeadlock):
		return ErrorSeverityMedium
	case IsNotFoundError(err) ||
		IsValidationError(err):
		return ErrorSeverityLow
	default:
		return ErrorSeverityMedium
	}
}

// ErrorRecoveryAction represents the recommended action for error recovery
type ErrorRecoveryAction int

const (
	ErrorRecoveryActionNone ErrorRecoveryAction = iota
	ErrorRecoveryActionRetry
	ErrorRecoveryActionReconnect
	ErrorRecoveryActionRestart
	ErrorRecoveryActionManualIntervention
)

func (era ErrorRecoveryAction) String() string {
	switch era {
	case ErrorRecoveryActionNone:
		return "none"
	case ErrorRecoveryActionRetry:
		return "retry"
	case ErrorRecoveryActionReconnect:
		return "reconnect"
	case ErrorRecoveryActionRestart:
		return "restart"
	case ErrorRecoveryActionManualIntervention:
		return "manual_intervention"
	default:
		return "unknown"
	}
}

// GetRecoveryAction determines the recommended recovery action for an error
func GetRecoveryAction(err error) ErrorRecoveryAction {
	if err == nil {
		return ErrorRecoveryActionNone
	}

	switch {
	case errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrQueryTimeout) ||
		errors.Is(err, ErrDeadlock) ||
		errors.Is(err, ErrTooManyConnections):
		return ErrorRecoveryActionRetry
	case errors.Is(err, ErrConnectionLost) ||
		errors.Is(err, ErrConnectionFailed):
		return ErrorRecoveryActionReconnect
	case errors.Is(err, ErrDataCorruption) ||
		errors.Is(err, ErrFileCorruption):
		return ErrorRecoveryActionRestart
	case errors.Is(err, ErrDiskFull) ||
		errors.Is(err, ErrInsufficientSpace) ||
		errors.Is(err, ErrPermissionDenied):
		return ErrorRecoveryActionManualIntervention
	case IsNotFoundError(err) ||
		IsValidationError(err):
		return ErrorRecoveryActionNone
	default:
		return ErrorRecoveryActionRetry
	}
}

// ErrorContext provides additional context for error handling
type ErrorContext struct {
	Operation  string
	Table      string
	Query      string
	Parameters []interface{}
	Timestamp  int64
	UserID     string
	SessionID  string
	RequestID  string
	Metadata   map[string]interface{}
}

// EnhancedError combines error with context and recovery information
type EnhancedError struct {
	Err            error
	Context        *ErrorContext
	Category       ErrorCategory
	Severity       ErrorSeverity
	RecoveryAction ErrorRecoveryAction
	Retryable      bool
	Temporary      bool
}

func (e *EnhancedError) Error() string {
	if e.Err == nil {
		return "unknown error"
	}
	return e.Err.Error()
}

func (e *EnhancedError) Unwrap() error {
	return e.Err
}

func (e *EnhancedError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e *EnhancedError) As(target interface{}) bool {
	return errors.As(e.Err, target)
}

// NewEnhancedError creates an enhanced error with full context
func NewEnhancedError(err error, context *ErrorContext) *EnhancedError {
	if err == nil {
		return nil
	}

	return &EnhancedError{
		Err:            err,
		Context:        context,
		Category:       CategorizeError(err),
		Severity:       GetErrorSeverity(err),
		RecoveryAction: GetRecoveryAction(err),
		Retryable:      isRetryableError(err),
		Temporary:      isTemporaryError(err),
	}
}

// Error aggregation for batch operations
type ErrorList struct {
	Errors []error
}

func (el *ErrorList) Error() string {
	if len(el.Errors) == 0 {
		return "no errors"
	}

	if len(el.Errors) == 1 {
		return el.Errors[0].Error()
	}

	var messages []string
	for i, err := range el.Errors {
		if i >= 5 { // Limit to first 5 errors
			messages = append(messages, fmt.Sprintf("... and %d more errors", len(el.Errors)-5))
			break
		}
		messages = append(messages, err.Error())
	}

	return fmt.Sprintf("multiple errors: %s", strings.Join(messages, "; "))
}

func (el *ErrorList) Add(err error) {
	if err != nil {
		el.Errors = append(el.Errors, err)
	}
}

func (el *ErrorList) HasErrors() bool {
	return len(el.Errors) > 0
}

func (el *ErrorList) Count() int {
	return len(el.Errors)
}

func (el *ErrorList) First() error {
	if len(el.Errors) == 0 {
		return nil
	}
	return el.Errors[0]
}

func (el *ErrorList) Last() error {
	if len(el.Errors) == 0 {
		return nil
	}
	return el.Errors[len(el.Errors)-1]
}

// NewErrorList creates a new error list
func NewErrorList() *ErrorList {
	return &ErrorList{
		Errors: make([]error, 0),
	}
}

// Utility functions for error handling in Go idioms
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	message := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", message, err)
}

// IsErrorType checks if an error is of a specific type
func IsErrorType[T error](err error) bool {
	var target T
	return errors.As(err, &target)
}

// ExtractError extracts a specific error type from an error chain
func ExtractError[T error](err error) (T, bool) {
	var target T
	if errors.As(err, &target) {
		return target, true
	}
	return target, false
}
