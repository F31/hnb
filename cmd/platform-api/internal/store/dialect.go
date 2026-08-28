package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// Driver is the symbolic name of a database backend that the store package can
// talk to. The empty string is treated as DriverPostgres so existing
// deployments that omit DB_DRIVER keep working unchanged.
type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverMySQL    Driver = "mysql"
)

// ErrUniqueViolation is the canonical error the store API surfaces when an
// INSERT collides with an existing UNIQUE constraint. Domain callers branch
// on errors.Is(err, ErrUniqueViolation) instead of inspecting lib/pq directly,
// so the same idempotent-replay logic keeps working once a MySQL backend
// ships. ErrUniqueViolation wraps the underlying driver error.
var ErrUniqueViolation = errors.New("unique constraint violation")

// ErrDialectNotImplemented is returned by stub MySQL store methods. Users who
// set DB_DRIVER=mysql see a clear operational error at the first real call,
// not a hard crash on startup, so the health check endpoint still works.
var ErrDialectNotImplemented = errors.New("dialect is not implemented for this driver; rebuild the binary with the appropriate driver support")

// Dialect abstracts the driver-specific behaviour the store needs:
//   - DriverName(): the *database/sql* driver name passed to sql.Open
//   - IsUniqueViolation(err): detect idempotent insert collisions
//   - NormalizeError(err): map driver-specific errors to canonical store errors
//
// The placeholder style ($1 vs ?) and the array column encoding are currently
// hard-wired to the Postgres schema every backend shares. Those become genuine
// per-backend concerns only if a schema migration forks SQL text per driver,
// which is out of scope for this change.
type Dialect interface {
	Driver() Driver
	DriverName() string
	IsUniqueViolation(err error) bool
	NormalizeError(err error) error
}

// newDialect resolves the dialect for a driver string. Unknown drivers
// surface a non-nil error rather than silently falling back, since a
// misconfigured DB_DRIVER would otherwise produce very confusing failures
// further inside when SQL text does not match the target engine.
func newDialect(driver Driver) (Dialect, error) {
	switch driver {
	case "", DriverPostgres:
		return postgresDialect{}, nil
	case DriverMySQL:
		return mysqlDialect{}, nil
	default:
		return nil, fmt.Errorf("unknown database driver %q (supported: %s, %s)",
			driver, DriverPostgres, DriverMySQL)
	}
}

// postgresDialect is the always-compiled Postgres implementation. The actual
// SQL stays confined to operations.go / clusters.go / targets.go; this is
// only the small behaviour surface that has to vary per engine.
type postgresDialect struct{}

func (postgresDialect) Driver() Driver     { return DriverPostgres }
func (postgresDialect) DriverName() string { return "postgres" }
func (postgresDialect) IsUniqueViolation(err error) bool {
	return isPgUniqueViolation(err)
}
func (postgresDialect) NormalizeError(err error) error { return normalizePostgresError(err) }

// mysqlDialect is the placeholder MySQL dialect. MySQL backends are selected by
// configuration today and surface a clear error on any type that has not yet
// been converted to a MySQL-native SQL form, so a misconfigured DB_DRIVER is
// caught at first call rather than silently producing wrong results.
type mysqlDialect struct{}

func (mysqlDialect) Driver() Driver                   { return DriverMySQL }
func (mysqlDialect) DriverName() string               { return "mysql" }
func (mysqlDialect) IsUniqueViolation(err error) bool { return false }
func (mysqlDialect) NormalizeError(err error) error   { return err }

// normalizePostgresError maps lib/pq error codes to canonical store errors.
// Callers can use errors.Is() against ErrUniqueViolation, ErrForeignKeyViolation,
// ErrSerializationFailure, or ErrDeadlockDetected.
func normalizePostgresError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pq.Error
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		return fmt.Errorf("%w: %s", ErrUniqueViolation, pgErr.Message)
	case "23503":
		return fmt.Errorf("%w: %s", ErrForeignKeyViolation, pgErr.Message)
	case "40001":
		return fmt.Errorf("%w: %s", ErrSerializationFailure, pgErr.Message)
	case "40P01":
		return fmt.Errorf("%w: %s", ErrDeadlockDetected, pgErr.Message)
	default:
		return err
	}
}

// New is the public store factory keyed on the configured Driver. It produces
// the abstract Store the HTTP layer wires through api.NewServer, with a
// concrete backend chosen at startup from configuration.
//
// Today only DriverPostgres is shipped with a working implementation; the
// MySQL backend returns a stub that implements Ping (so misconfigurations can
// hit diagnostics via the health check) but every other method surfaces
// ErrMySQLNotImplemented.
func New(db *sql.DB, driver Driver) (Store, error) {
	d, err := newDialect(driver)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	switch d.Driver() {
	case DriverPostgres:
		return NewPGStore(db), nil
	case DriverMySQL:
		return newMySQLStubStore(db), nil
	default:
		return nil, fmt.Errorf("store: unsupported driver %q", driver)
	}
}
