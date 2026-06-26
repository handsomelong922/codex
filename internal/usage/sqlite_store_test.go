package usage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSQLiteStoreRestrictsParentDirectoryAndDatabaseFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode assertions are not portable on windows")
	}

	ctx := context.Background()
	parentDir := filepath.Join(t.TempDir(), "usage-data")
	if err := os.Mkdir(parentDir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	dbPath := filepath.Join(parentDir, "usage.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	parentInfo, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("Stat(parentDir) error = %v", err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("parent directory mode = %#o, want 0700", got)
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat(dbPath) error = %v", err)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("db file mode = %#o, want 0600", got)
	}

	record := Record{
		ID:        "permission-record",
		Timestamp: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		APIKey:    "permission-api",
		Model:     "permission-model",
		Tokens:    TokenStats{TotalTokens: 1},
	}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got := idsOf(usage["permission-api"]["permission-model"]); len(got) != 1 || got[0] != "permission-record" {
		t.Fatalf("ids = %v, want [permission-record]", got)
	}
}

func TestSQLiteStoreBareFilenameDoesNotChmodWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode assertions are not portable on windows")
	}

	workDir := t.TempDir()
	if err := os.Chmod(workDir, 0o755); err != nil {
		t.Fatalf("Chmod(workDir) error = %v", err)
	}
	t.Chdir(workDir)

	store, err := NewSQLiteStore("usage.db")
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	workDirInfo, err := os.Stat(workDir)
	if err != nil {
		t.Fatalf("Stat(workDir) error = %v", err)
	}
	if got := workDirInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("working directory mode = %#o, want 0755", got)
	}
	dbInfo, err := os.Stat(filepath.Join(workDir, "usage.db"))
	if err != nil {
		t.Fatalf("Stat(usage.db) error = %v", err)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("db file mode = %#o, want 0600", got)
	}
}

func TestSQLiteStoreInsertQueryAndDelete(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	at := time.Date(2026, 5, 2, 12, 0, 0, 123, time.UTC)
	record := Record{
		ID:                 "record-1",
		Timestamp:          at,
		APIKey:             "sk-user-a",
		Provider:           "openai",
		Model:              "gpt-5.4",
		Source:             "user@example.com",
		AuthIndex:          "0",
		AuthType:           "apikey",
		Endpoint:           "POST /v1/chat/completions",
		RequestID:          "request-1",
		LatencyMs:          1800,
		FirstByteLatencyMs: 320,
		GenerationMs:       1480,
		ThinkingEffort:     "high",
		ServiceTier:        "priority",
		Tokens: TokenStats{
			InputTokens:     300,
			OutputTokens:    500,
			ReasoningTokens: 60,
			CachedTokens:    100,
			TotalTokens:     860,
		},
		Failed:         false,
		FailStatusCode: 0,
		FailBody:       "",
	}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	details := usage["sk-user-a"]["gpt-5.4"]
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	got := details[0]
	if got.ID != "record-1" || !got.Timestamp.Equal(at) || got.LatencyMs != 1800 || got.FirstByteLatencyMs != 320 || got.GenerationMs != 1480 || got.ThinkingEffort != "high" {
		t.Fatalf("detail = %+v", got)
	}
	if got.ServiceTier != "priority" {
		t.Fatalf("service tier = %q, want priority", got.ServiceTier)
	}
	if got.Source != "user@example.com" || got.AuthIndex != "0" || got.Failed {
		t.Fatalf("detail metadata = %+v", got)
	}
	if got.Tokens != record.Tokens {
		t.Fatalf("tokens = %+v, want %+v", got.Tokens, record.Tokens)
	}

	result, err := store.Delete(ctx, []string{"record-1", "missing-id"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if result.Deleted != 1 || len(result.Missing) != 1 || result.Missing[0] != "missing-id" {
		t.Fatalf("Delete() = %+v, want deleted=1 missing=[missing-id]", result)
	}

	usage, err = store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() after delete error = %v", err)
	}
	if len(usage) != 0 {
		t.Fatalf("record was not deleted: %+v", usage)
	}
}

func TestSQLiteStoreQueryRange(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	first := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	for _, record := range []Record{
		{ID: "old", Timestamp: first, APIKey: "api", Model: "model", Tokens: TokenStats{TotalTokens: 1}},
		{ID: "new", Timestamp: second, APIKey: "api", Model: "model", Tokens: TokenStats{TotalTokens: 1}},
	} {
		if err := store.Insert(ctx, record); err != nil {
			t.Fatalf("Insert(%s) error = %v", record.ID, err)
		}
	}

	start := second
	usage, err := store.Query(ctx, QueryRange{Start: &start})
	if err != nil {
		t.Fatalf("Query(start) error = %v", err)
	}
	if got := idsOf(usage["api"]["model"]); len(got) != 1 || got[0] != "new" {
		t.Fatalf("start ids = %v, want [new]", got)
	}

	end := second
	usage, err = store.Query(ctx, QueryRange{End: &end})
	if err != nil {
		t.Fatalf("Query(end) error = %v", err)
	}
	if got := idsOf(usage["api"]["model"]); len(got) != 1 || got[0] != "old" {
		t.Fatalf("end ids = %v, want [old]", got)
	}
}

func TestSQLiteStoreIgnoresZeroRangeBounds(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	historical := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	record := Record{ID: "historical", Timestamp: historical, APIKey: "api", Model: "model", Tokens: TokenStats{TotalTokens: 1}}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	zero := time.Time{}
	for _, tc := range []struct {
		name string
		rng  QueryRange
	}{
		{name: "start", rng: QueryRange{Start: &zero}},
		{name: "end", rng: QueryRange{End: &zero}},
		{name: "both", rng: QueryRange{Start: &zero, End: &zero}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage, err := store.Query(ctx, tc.rng)
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			if got := idsOf(usage["api"]["model"]); len(got) != 1 || got[0] != "historical" {
				t.Fatalf("ids = %v, want [historical]", got)
			}
		})
	}
}

func TestSQLiteStoreGroupsByEndpointWhenAPIKeyMissing(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	record := Record{
		ID:        "endpoint-record",
		Timestamp: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		Provider:  "claude",
		Model:     "claude-sonnet-4-6",
		Endpoint:  "POST /v1/messages",
		Tokens:    TokenStats{TotalTokens: 1},
	}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(usage) != 1 {
		t.Fatalf("usage keys = %+v, want only endpoint key", usage)
	}
	details := usage["POST /v1/messages"]["claude-sonnet-4-6"]
	if len(details) != 1 || details[0].ID != "endpoint-record" {
		t.Fatalf("endpoint details = %+v, want endpoint-record", details)
	}
	if _, ok := usage["claude"]; ok {
		t.Fatalf("provider key was used instead of endpoint: %+v", usage)
	}
}

func TestSQLiteStoreNormalizesInsertedRecord(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	record := Record{
		ID:                 "normalized-record",
		Timestamp:          time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		APIKey:             "api",
		Model:              " ",
		LatencyMs:          -1,
		FirstByteLatencyMs: -2,
		GenerationMs:       -3,
		Tokens: TokenStats{
			InputTokens:     2,
			OutputTokens:    3,
			ReasoningTokens: 4,
			CachedTokens:    5,
		},
		Failed: true,
	}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	negativeTokensRecord := Record{
		ID:        "negative-token-record",
		Timestamp: time.Date(2026, 5, 2, 12, 1, 0, 0, time.UTC),
		APIKey:    "api",
		Model:     "negative-token-model",
		Tokens: TokenStats{
			InputTokens:     -2,
			OutputTokens:    -3,
			ReasoningTokens: -4,
			CachedTokens:    -5,
			TotalTokens:     -6,
		},
	}
	if err := store.Insert(ctx, negativeTokensRecord); err != nil {
		t.Fatalf("Insert(negative tokens) error = %v", err)
	}

	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	details := usage["api"]["unknown"]
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	got := details[0]
	if got.LatencyMs != 0 || got.FirstByteLatencyMs != 0 || got.GenerationMs != 0 {
		t.Fatalf("latencies = (%d, %d, %d), want all zero", got.LatencyMs, got.FirstByteLatencyMs, got.GenerationMs)
	}
	if got.Tokens.TotalTokens != 9 {
		t.Fatalf("total tokens = %d, want 9", got.Tokens.TotalTokens)
	}
	if !got.Failed {
		t.Fatalf("failed = false, want true")
	}
	negativeTokenDetails := usage["api"]["negative-token-model"]
	if len(negativeTokenDetails) != 1 {
		t.Fatalf("negative token details len = %d, want 1", len(negativeTokenDetails))
	}
	if negativeTokenDetails[0].Tokens != (TokenStats{}) {
		t.Fatalf("negative tokens = %+v, want all zero", negativeTokenDetails[0].Tokens)
	}
}

func TestSQLiteStorePersistsFailureMetadata(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteStore(t)

	record := Record{
		ID:             "failure-record",
		Timestamp:      time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
		APIKey:         "sk-fail",
		Model:          "gpt-5.4",
		ServiceTier:    "default",
		Failed:         true,
		FailStatusCode: 429,
		FailBody:       "rate limited",
	}
	if err := store.Insert(ctx, record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	details := usage["sk-fail"]["gpt-5.4"]
	if len(details) != 1 {
		t.Fatalf("details len = %d, want 1", len(details))
	}
	got := details[0]
	if !got.Failed || got.FailStatusCode != 429 || got.FailBody != "rate limited" || got.ServiceTier != "default" {
		t.Fatalf("failure detail = %+v", got)
	}
}

func TestSQLiteStoreMigratesLegacySchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "usage.db")

	// Create a database carrying the original schema (without service_tier,
	// fail_status_code, fail_body) and a legacy row, then reopen it through
	// NewSQLiteStore to exercise the ALTER-based migration path.
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := legacy.ExecContext(ctx, `CREATE TABLE usage_records (
	id TEXT PRIMARY KEY,
	timestamp TEXT NOT NULL,
	api_key TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	auth_index TEXT NOT NULL DEFAULT '',
	auth_type TEXT NOT NULL DEFAULT '',
	endpoint TEXT NOT NULL DEFAULT '',
	request_id TEXT NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL DEFAULT 0,
	first_byte_latency_ms INTEGER NOT NULL DEFAULT 0,
	generation_ms INTEGER NOT NULL DEFAULT 0,
	thinking_effort TEXT NOT NULL DEFAULT '',
	input_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	reasoning_tokens INTEGER NOT NULL DEFAULT 0,
	cached_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	failed INTEGER NOT NULL DEFAULT 0
)`); err != nil {
		t.Fatalf("create legacy schema error = %v", err)
	}
	if _, err := legacy.ExecContext(ctx, `INSERT INTO usage_records
	(id, timestamp, api_key, model, total_tokens, failed)
	VALUES ('legacy-1', '2026-05-02T12:00:00.000000000Z', 'sk-legacy', 'gpt-5.4', 5, 0)`); err != nil {
		t.Fatalf("insert legacy row error = %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db error = %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Legacy row is still readable with defaulted new columns.
	usage, err := store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() legacy error = %v", err)
	}
	legacyDetails := usage["sk-legacy"]["gpt-5.4"]
	if len(legacyDetails) != 1 {
		t.Fatalf("legacy details len = %d, want 1", len(legacyDetails))
	}
	if legacyDetails[0].ServiceTier != "" || legacyDetails[0].FailStatusCode != 0 || legacyDetails[0].FailBody != "" {
		t.Fatalf("legacy detail new columns = %+v, want zero values", legacyDetails[0])
	}

	// New rows can use the migrated columns.
	if err := store.Insert(ctx, Record{
		ID:             "migrated-1",
		Timestamp:      time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		APIKey:         "sk-legacy",
		Model:          "gpt-5.4",
		ServiceTier:    "priority",
		Failed:         true,
		FailStatusCode: 500,
		FailBody:       "boom",
	}); err != nil {
		t.Fatalf("Insert() migrated error = %v", err)
	}
	usage, err = store.Query(ctx, QueryRange{})
	if err != nil {
		t.Fatalf("Query() migrated error = %v", err)
	}
	var migrated *RequestDetail
	for i := range usage["sk-legacy"]["gpt-5.4"] {
		if usage["sk-legacy"]["gpt-5.4"][i].ID == "migrated-1" {
			migrated = &usage["sk-legacy"]["gpt-5.4"][i]
		}
	}
	if migrated == nil {
		t.Fatalf("migrated row not found: %+v", usage)
	}
	if migrated.ServiceTier != "priority" || migrated.FailStatusCode != 500 || migrated.FailBody != "boom" {
		t.Fatalf("migrated detail = %+v", migrated)
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func idsOf(details []RequestDetail) []string {
	ids := make([]string, 0, len(details))
	for _, detail := range details {
		ids = append(ids, detail.ID)
	}
	return ids
}
