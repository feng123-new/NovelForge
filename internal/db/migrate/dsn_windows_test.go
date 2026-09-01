//go:build windows

package migrate

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDSNWindowsDriveUsesEmptyURIAuthority(t *testing.T) {
	t.Parallel()
	dsn, err := DSN(filepath.Join(t.TempDir(), "project ? #.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		t.Fatalf("invalid Windows file URI: scheme=%q host=%q path=%q dsn=%q", parsed.Scheme, parsed.Host, parsed.Path, dsn)
	}
}
