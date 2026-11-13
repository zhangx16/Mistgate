package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistgate/mistgate/internal/panel/provision"
	"github.com/mistgate/mistgate/internal/release"
)

// Alpine's optional local bundle is explicitly pinned to its own public key;
// it never replaces the official updater's trust root or Ubuntu/Debian bundles.
type alpineNodeBinaries struct{ dir, key string }

func alpineNodeBinariesFromEnv() provision.BinarySource {
	dir := os.Getenv("MISTGATE_ALPINE_BUNDLE_DIR")
	if dir == "" {
		return nil
	}
	return &alpineNodeBinaries{dir: dir, key: os.Getenv("MISTGATE_ALPINE_BUNDLE_KEY")}
}

func (b *alpineNodeBinaries) OpenNodeBinary(ctx context.Context, goos, arch string) (io.ReadCloser, int64, string, error) {
	fail := func() (io.ReadCloser, int64, string, error) {
		return nil, 0, "", errors.New("Alpine agent bundle is unavailable or untrusted")
	}
	if ctx.Err() != nil {
		return nil, 0, "", ctx.Err()
	}
	key, err := base64.StdEncoding.DecodeString(b.key)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return fail()
	}
	manifest, err := os.ReadFile(filepath.Join(b.dir, "manifest.json"))
	if err != nil {
		return fail()
	}
	signature, err := os.ReadFile(filepath.Join(b.dir, "manifest.sig"))
	if err != nil {
		return fail()
	}
	m, err := release.Verify(ed25519.PublicKey(key), manifest, signature)
	if err != nil || m.Expires <= time.Now().Unix() {
		return fail()
	}
	entry, err := m.FileFor(goos, arch)
	if err != nil {
		return fail()
	}
	if filepath.Base(entry.Name) != entry.Name || strings.Contains(entry.Name, "..") {
		return fail()
	}
	f, err := os.Open(filepath.Join(b.dir, entry.Name))
	if err != nil {
		return fail()
	}
	if err := release.VerifyFile(entry, f); err != nil {
		f.Close()
		return fail()
	}
	if _, err := hex.DecodeString(entry.SHA256); err != nil {
		f.Close()
		return fail()
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return fail()
	}
	return f, entry.Size, entry.SHA256, nil
}
