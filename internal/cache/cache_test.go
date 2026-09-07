package cache

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func open(t *testing.T, root string, mutate func(*Options)) (*Cache, *clock) {
	t.Helper()
	ck := &clock{now: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
	opts := Options{Root: root, Profile: "acme", Now: ck.Now}
	if mutate != nil {
		mutate(&opts)
	}
	c, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	return c, ck
}

func TestDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	if d, _ := Dir(); d != filepath.Join("/tmp/xdg", "figctl") {
		t.Fatalf("dir = %s", d)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "/home/me")
	if d, _ := Dir(); d != filepath.Join("/home/me", ".cache", "figctl") {
		t.Fatalf("dir = %s", d)
	}
	if _, err := Open(Options{Profile: "a/b"}); err == nil {
		t.Fatal("profile with a slash must be rejected")
	}
}

func TestKey(t *testing.T) {
	q := url.Values{"depth": {"2"}, "ids": {"1:2,1:3"}}
	if k := Key("nodes", q); k != "nodes?depth=2&ids=1%3A2%2C1%3A3" {
		t.Fatalf("key = %s", k)
	}
	if k := Key("file", nil); k != "file" {
		t.Fatalf("key = %s", k)
	}
	if k := BlobKey("1:2", "png", 2, "v1"); k != "render/1:2/png/2/v1" {
		t.Fatalf("blob key = %s", k)
	}
}

func TestHitMissRefreshNoCache(t *testing.T) {
	root := t.TempDir()
	c, ck := open(t, root, nil)
	stamp := Stamp{Version: "1", LastTouchedAt: "t1"}

	if _, ok, err := c.Get("FILE", "file", stamp); ok || err != nil {
		t.Fatalf("expected miss, ok=%v err=%v", ok, err)
	}
	if err := c.Put("FILE", "file", stamp, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	data, ok, err := c.Get("FILE", "file", stamp)
	if err != nil || !ok || string(data) != `{"a":1}` {
		t.Fatalf("expected hit: %q %v %v", data, ok, err)
	}
	if _, ok, _ := c.Get("FILE", "file", Stamp{Version: "2", LastTouchedAt: "t2"}); ok {
		t.Fatal("stale stamp must miss")
	}
	if _, ok, _ := c.Get("FILE", "file", stamp); ok {
		t.Fatal("stale entry should have been removed")
	}
	if err := c.Put("FILE", "file", stamp, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, "acme", "FILE"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("file dir perms: %v %v", info.Mode(), err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "acme", "FILE", entriesDir))
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	if einfo, _ := entries[0].Info(); einfo.Mode().Perm() != 0o600 {
		t.Fatalf("entry perms: %v", einfo.Mode())
	}

	refresh, _ := open(t, root, func(o *Options) { o.Refresh = true; o.Now = ck.Now })
	if _, ok, _ := refresh.Get("FILE", "file", stamp); ok {
		t.Fatal("--refresh must miss")
	}
	if err := refresh.Put("FILE", "file", stamp, []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	if data, ok, _ := c.Get("FILE", "file", stamp); !ok || string(data) != `{"a":2}` {
		t.Fatalf("--refresh must rewrite: %q %v", data, ok)
	}

	none, _ := open(t, root, func(o *Options) { o.NoCache = true; o.Now = ck.Now })
	if _, ok, _ := none.Get("FILE", "file", stamp); ok {
		t.Fatal("--no-cache must miss")
	}
	if err := none.Put("FILE", "file", stamp, []byte(`{"a":3}`)); err != nil {
		t.Fatal(err)
	}
	if data, _, _ := c.Get("FILE", "file", stamp); string(data) != `{"a":2}` {
		t.Fatalf("--no-cache must not write: %q", data)
	}
	if err := none.Put("OTHER", "file", stamp, []byte(`x`)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "acme", "OTHER")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("--no-cache must not create directories")
	}

	other, _ := open(t, root, func(o *Options) { o.Profile = "other" })
	if _, ok, _ := other.Get("FILE", "file", stamp); ok {
		t.Fatal("profiles must be isolated")
	}

	if _, ok, _ := c.GetBlob("FILE", BlobKey("1:2", "png", 2, "1")); ok {
		t.Fatal("blob miss expected")
	}
	if err := c.PutBlob("FILE", BlobKey("1:2", "png", 2, "1"), []byte("png")); err != nil {
		t.Fatal(err)
	}
	if data, ok, _ := c.GetBlob("FILE", BlobKey("1:2", "png", 2, "1")); !ok || string(data) != "png" {
		t.Fatalf("blob hit expected: %q %v", data, ok)
	}
}

func TestValidateTTL(t *testing.T) {
	root := t.TempDir()
	c, ck := open(t, root, func(o *Options) { o.TTL = time.Minute })
	calls := 0
	stamp := Stamp{Version: "1", LastTouchedAt: "t1"}
	fetch := func(context.Context) (Stamp, error) {
		calls++
		return stamp, nil
	}
	ctx := context.Background()
	got, err := c.Validate(ctx, "FILE", fetch)
	if err != nil || got != stamp || calls != 1 {
		t.Fatalf("first validate: %+v %v calls=%d", got, err, calls)
	}
	ck.now = ck.now.Add(30 * time.Second)
	if got, _ := c.Validate(ctx, "FILE", fetch); got != stamp || calls != 1 {
		t.Fatalf("within ttl should not fetch: calls=%d", calls)
	}
	ck.now = ck.now.Add(31 * time.Second)
	stamp = Stamp{Version: "2", LastTouchedAt: "t2"}
	if got, _ := c.Validate(ctx, "FILE", fetch); got != stamp || calls != 2 {
		t.Fatalf("after ttl should fetch: %+v calls=%d", got, calls)
	}
	if _, ok, _ := c.Get("FILE", "file", Stamp{Version: "1", LastTouchedAt: "t1"}); ok {
		t.Fatal("old stamp should miss")
	}

	refresh, _ := open(t, root, func(o *Options) { o.Refresh = true; o.Now = ck.Now; o.TTL = time.Minute })
	if _, err := refresh.Validate(ctx, "FILE", fetch); err != nil || calls != 3 {
		t.Fatalf("--refresh must fetch: calls=%d err=%v", calls, err)
	}
	none, _ := open(t, root, func(o *Options) { o.NoCache = true; o.Now = ck.Now })
	if _, err := none.Validate(ctx, "FILE", fetch); err != nil || calls != 4 {
		t.Fatalf("--no-cache must fetch: calls=%d err=%v", calls, err)
	}
	failing := func(context.Context) (Stamp, error) { return Stamp{}, errors.New("boom") }
	ck.now = ck.now.Add(2 * time.Minute)
	if _, err := c.Validate(ctx, "FILE", failing); err == nil {
		t.Fatal("fetch errors must propagate")
	}

	if err := c.SetTooLarge("FILE", true); err != nil {
		t.Fatal(err)
	}
	state, err := c.FileState("FILE")
	if err != nil || !state.TooLarge || state.Stamp != stamp {
		t.Fatalf("state: %+v %v", state, err)
	}
}

func TestScanAndClear(t *testing.T) {
	root := t.TempDir()
	c, ck := open(t, root, nil)
	stamp := Stamp{Version: "1", LastTouchedAt: "t1"}
	ctx := context.Background()
	if _, err := c.Validate(ctx, "FILE", func(context.Context) (Stamp, error) { return stamp, nil }); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("FILE", "file", stamp, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("FILE", "nodes?ids=1", stamp, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.PutBlob("FILE", "render/1:2/png/2/1", []byte("png")); err != nil {
		t.Fatal(err)
	}
	other, _ := open(t, root, func(o *Options) { o.Profile = "other"; o.Now = ck.Now })
	if err := other.Put("OTHER", "file", stamp, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}

	status, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Profiles) != 2 || status.Profiles[0].Name != "acme" || status.Bytes <= 0 {
		t.Fatalf("status: %+v", status)
	}
	file := status.Profiles[0].Files[0]
	if file.Key != "FILE" || len(file.Entries) != 2 || file.Entries[0].Key != "file" || file.Entries[0].Version != "1" || file.Blobs != 1 || file.Version != "1" || file.CheckedAt == nil {
		t.Fatalf("file status: %+v", file)
	}
	if file.Entries[0].FetchedAt != ck.now {
		t.Fatalf("fetchedAt = %v", file.Entries[0].FetchedAt)
	}

	removed, err := Clear(root, "acme", "NOPE")
	if err != nil || removed != 0 {
		t.Fatalf("clear unknown file: %d %v", removed, err)
	}
	removed, err = Clear(root, "acme", "FILE")
	if err != nil || removed != file.Bytes {
		t.Fatalf("clear file: %d %v", removed, err)
	}
	status, _ = Scan(root)
	if len(status.Profiles[0].Files) != 0 || len(status.Profiles[1].Files) != 1 {
		t.Fatalf("after clear: %+v", status)
	}
	if _, err := Clear(root, "", ""); err != nil {
		t.Fatal(err)
	}
	status, _ = Scan(root)
	if len(status.Profiles) != 0 || status.Bytes != 0 {
		t.Fatalf("after clear all: %+v", status)
	}
	if empty, err := Scan(filepath.Join(root, "missing")); err != nil || len(empty.Profiles) != 0 {
		t.Fatalf("missing root: %+v %v", empty, err)
	}
	if FormatBytes(512) != "512 B" || FormatBytes(1536) != "1.5 KiB" || FormatBytes(3*1024*1024) != "3.0 MiB" {
		t.Fatal("FormatBytes")
	}
}
