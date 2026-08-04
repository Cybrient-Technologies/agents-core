package crypto

import (
	"strings"
	"testing"
)

// keyHex is a fixed test key: 32 bytes of 0xab as 64 hex chars.
const keyHex = "abababababababab" + "abababababababab" + "abababababababab" + "abababababababab"

// TestDecryptKnownVector checks decryption of a fixed AES-256-GCM ciphertext,
// produced externally with the key above and plaintext "hello-secret-key-123".
func TestDecryptKnownVector(t *testing.T) {
	const phpCiphertext = "VaCwVZV5x4O7Haex57PTDD77Y6DchGGoRsWRYxwUQXaFrDSpCaWLDzBLXYTUI1Fa"
	c, err := New(keyHex)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.Decrypt(phpCiphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "hello-secret-key-123" {
		t.Fatalf("got %q, want %q", got, "hello-secret-key-123")
	}
}

func TestRoundTrip(t *testing.T) {
	c, err := New(keyHex)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, pt := range []string{"", "sk-ant-abc123", strings.Repeat("x", 500)} {
		enc, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if dec != pt {
			t.Fatalf("round-trip mismatch: got %q want %q", dec, pt)
		}
	}
}

func TestBadKey(t *testing.T) {
	if _, err := New("zz"); err == nil {
		t.Fatal("expected error for non-hex key")
	}
	if _, err := New("abcd"); err == nil {
		t.Fatal("expected error for short key")
	}
}
