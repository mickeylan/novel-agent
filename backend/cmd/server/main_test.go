package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type testReadCloser struct {
	reader   io.Reader
	closeErr error
}

func (r testReadCloser) Read(buffer []byte) (int, error) { return r.reader.Read(buffer) }
func (r testReadCloser) Close() error                    { return r.closeErr }

func TestReadEnvironmentFileRejectsReadAndCloseFailures(t *testing.T) {
	readFailure := testReadCloser{reader: errorReader{}, closeErr: nil}
	if data, err := readEnvironmentFile(readFailure); err == nil || data != nil || err.Error() != "failed to read environment file" {
		t.Fatalf("unexpected read failure result: data=%q err=%v", data, err)
	}
	closeFailure := testReadCloser{reader: strings.NewReader("KEY=value"), closeErr: errors.New("secret close detail")}
	if data, err := readEnvironmentFile(closeFailure); err == nil || data != nil || err.Error() != "failed to read environment file" {
		t.Fatalf("unexpected close failure result: data=%q err=%v", data, err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("secret read detail") }

func TestResolveConfigPathDistinguishesUnsetFromInvalidEmpty(t *testing.T) {
	original, existed := os.LookupEnv("CONFIG_PATH")
	_ = os.Unsetenv("CONFIG_PATH")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("CONFIG_PATH", original)
		} else {
			_ = os.Unsetenv("CONFIG_PATH")
		}
	})
	if path, err := resolveConfigPath(); err != nil || path != "config.yaml" {
		t.Fatalf("unexpected default config path: path=%q err=%v", path, err)
	}
	for name, value := range map[string]string{
		"empty":       "",
		"too long":    strings.Repeat("x", 4097),
		"control":     "config\n.yaml",
		"format rune": "config\u200b.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CONFIG_PATH", value)
			path, err := resolveConfigPath()
			if err == nil || path != "" || err.Error() != "CONFIG_PATH is invalid" {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if value != "" && strings.Contains(err.Error(), value) {
				t.Fatalf("config path error reflected input: %v", err)
			}
		})
	}
	t.Setenv("CONFIG_PATH", "custom.yaml")
	if path, err := resolveConfigPath(); err != nil || path != "custom.yaml" {
		t.Fatalf("unexpected explicit config path: path=%q err=%v", path, err)
	}
	if validConfigPath(string([]byte{'c', 0xff, 'f'})) {
		t.Fatal("invalid UTF-8 config path was accepted")
	}
}

func TestConfigureGinModeAppliesValidatedMode(t *testing.T) {
	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })
	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode, gin.TestMode} {
		if err := configureGinMode(mode); err != nil {
			t.Fatalf("configureGinMode(%q): %v", mode, err)
		}
		if actual := gin.Mode(); actual != mode {
			t.Fatalf("gin mode = %q, want %q", actual, mode)
		}
	}
	if err := configureGinMode("invalid"); err == nil {
		t.Fatal("invalid Gin mode was accepted")
	}
}

func TestLoadDotEnvIsBoundedStrictAndPreservesExistingEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	t.Setenv("NOVEL_AGENT_EXISTING_TEST", "process-value")
	defer os.Unsetenv("NOVEL_AGENT_NEW_TEST")
	if err := os.WriteFile(path, []byte("# comment\nNOVEL_AGENT_EXISTING_TEST=file-value\nNOVEL_AGENT_NEW_TEST= new-value \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDotEnv(path)
	if err != nil || !loaded {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if value := os.Getenv("NOVEL_AGENT_EXISTING_TEST"); value != "process-value" {
		t.Fatalf("existing environment overwritten: %q", value)
	}
	if value := os.Getenv("NOVEL_AGENT_NEW_TEST"); value != "new-value" {
		t.Fatalf("new environment not loaded: %q", value)
	}
}

func TestLoadDotEnvPreservesDatabasePasswordSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const password = "  password with spaces  "
	original, existed := os.LookupEnv("DB_PASSWORD")
	_ = os.Unsetenv("DB_PASSWORD")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("DB_PASSWORD", original)
		} else {
			_ = os.Unsetenv("DB_PASSWORD")
		}
	})
	if err := os.WriteFile(path, []byte("DB_PASSWORD="+password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDotEnv(path)
	if err != nil || !loaded {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if actual := os.Getenv("DB_PASSWORD"); actual != password {
		t.Fatalf("database password whitespace changed: got %q", actual)
	}
}

func TestLoadDotEnvRejectsInvalidUTF8BeforeApplyingVariables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const key = "NOVEL_AGENT_INVALID_UTF8_TEST"
	original, existed := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	data := append([]byte(key+"=valid\nSECOND="), 0xff, 0xfe)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDotEnv(path)
	if err == nil || loaded || err.Error() != "environment file must contain valid UTF-8" {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if _, exists := os.LookupEnv(key); exists {
		t.Fatal("environment variable was applied before UTF-8 validation")
	}
}

func TestLoadDotEnvRejectsMalformedAndOversizedFilesWithoutEchoingValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	secretLikeValue := "credential-shaped-private-value"
	for name, content := range map[string]string{
		"missing equals":    secretLikeValue,
		"invalid name":      "INVALID-NAME=" + secretLikeValue,
		"Unicode format":    "VALID_NAME=token\u200bhidden",
		"Unicode separator": "VALID_NAME=token\u2028hidden",
		"oversized":         strings.Repeat("X", int(maxDotEnvBytes)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadDotEnv(path)
			if err == nil || loaded {
				t.Fatalf("loaded=%v err=%v", loaded, err)
			}
			if strings.Contains(err.Error(), secretLikeValue) || strings.Contains(err.Error(), path) {
				t.Fatalf("environment path or value leaked: %v", err)
			}
		})
	}
}

func TestLoadDotEnvRejectsDuplicateVariablesAndDoesNotPartiallyApply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const firstKey = "NOVEL_AGENT_ATOMIC_FIRST_TEST"
	_ = os.Unsetenv(firstKey)
	t.Cleanup(func() {
		_ = os.Unsetenv(firstKey)
	})

	for name, content := range map[string]string{
		"invalid later line": firstKey + "=first\ninvalid-line",
		"duplicate key":      firstKey + "=first\n" + firstKey + "=second",
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadDotEnv(path)
			if err == nil || loaded {
				t.Fatalf("loaded=%v err=%v", loaded, err)
			}
			if _, exists := os.LookupEnv(firstKey); exists {
				t.Fatal("environment file was partially applied")
			}
		})
	}
}

func TestLoadDotEnvRollsBackAppliedVariablesWhenLaterSetFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const firstKey = "NOVEL_AGENT_ROLLBACK_FIRST_TEST"
	const secondKey = "NOVEL_AGENT_ROLLBACK_SECOND_TEST"
	_ = os.Unsetenv(firstKey)
	_ = os.Unsetenv(secondKey)
	originalSet, originalUnset := setEnvironmentVariable, unsetEnvironmentVariable
	t.Cleanup(func() {
		setEnvironmentVariable, unsetEnvironmentVariable = originalSet, originalUnset
		_ = os.Unsetenv(firstKey)
		_ = os.Unsetenv(secondKey)
	})
	setEnvironmentVariable = func(key, value string) error {
		if key == secondKey {
			return errors.New("simulated set failure containing secret material")
		}
		return os.Setenv(key, value)
	}
	unsetEnvironmentVariable = os.Unsetenv
	if err := os.WriteFile(path, []byte(firstKey+"=first\n"+secondKey+"=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDotEnv(path)
	if err == nil || loaded || err.Error() != "failed to apply environment file" {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if _, exists := os.LookupEnv(firstKey); exists {
		t.Fatal("previously applied environment variable was not rolled back")
	}
}

func TestLoadDotEnvReportsRollbackFailureWithoutLeakingDetails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	const firstKey = "NOVEL_AGENT_ROLLBACK_FAILURE_FIRST_TEST"
	const secondKey = "NOVEL_AGENT_ROLLBACK_FAILURE_SECOND_TEST"
	_ = os.Unsetenv(firstKey)
	_ = os.Unsetenv(secondKey)
	originalSet, originalUnset := setEnvironmentVariable, unsetEnvironmentVariable
	t.Cleanup(func() {
		setEnvironmentVariable, unsetEnvironmentVariable = originalSet, originalUnset
		_ = os.Unsetenv(firstKey)
		_ = os.Unsetenv(secondKey)
	})
	setEnvironmentVariable = func(key, value string) error {
		if key == secondKey {
			return errors.New("set-secret-detail")
		}
		return os.Setenv(key, value)
	}
	unsetEnvironmentVariable = func(string) error { return errors.New("unset-secret-detail") }
	if err := os.WriteFile(path, []byte(firstKey+"=first\n"+secondKey+"=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDotEnv(path)
	if err == nil || loaded || err.Error() != "failed to apply and roll back environment file" {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), path) {
		t.Fatalf("rollback error leaked details: %v", err)
	}
}

func TestLoadDotEnvIgnoresMissingFile(t *testing.T) {
	loaded, err := loadDotEnv(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil || loaded {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
}
