package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistgate/mistgate/internal/release"
)

func TestAlpineBundleRejectsTamperingAndWrongKey(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	body := []byte("test node binary")
	sum := sha256.Sum256(body)
	m := release.Manifest{Schema: release.Schema, Version: "v0.1.86-alpine.1", Built: time.Now().Unix(), Expires: time.Now().Add(time.Hour).Unix(), Files: []release.File{{OS: "linux", Arch: "amd64", Name: "mistgate-node-linux-amd64", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}}}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	os.WriteFile(filepath.Join(dir, "manifest.sig"), release.Sign(key, raw), 0600)
	file := filepath.Join(dir, "mistgate-node-linux-amd64")
	os.WriteFile(file, body, 0755)
	b := &alpineNodeBinaries{dir: dir, key: base64.StdEncoding.EncodeToString(pub)}
	f, _, _, err := b.OpenNodeBinary(context.Background(), "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.WriteFile(file, []byte("tampered"), 0755)
	if f, _, _, err := b.OpenNodeBinary(context.Background(), "linux", "amd64"); err == nil {
		f.Close()
		t.Fatal("tampered binary accepted")
	}
	os.WriteFile(file, body, 0755)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	b.key = base64.StdEncoding.EncodeToString(other)
	if f, _, _, err := b.OpenNodeBinary(context.Background(), "linux", "amd64"); err == nil {
		f.Close()
		t.Fatal("wrong key accepted")
	}
	b.key = base64.StdEncoding.EncodeToString(pub)
	m.Expires = time.Now().Add(-time.Hour).Unix()
	m.Built = m.Expires - 3600
	raw, _ = json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	os.WriteFile(filepath.Join(dir, "manifest.sig"), release.Sign(key, raw), 0600)
	if f, _, _, err := b.OpenNodeBinary(context.Background(), "linux", "amd64"); err == nil {
		f.Close()
		t.Fatal("expired bundle accepted")
	}
}
