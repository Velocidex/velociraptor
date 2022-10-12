package zip

import (
	"bytes"
	"io"
	"testing"
)

func TestStandardEncryptionRoundtrip(t *testing.T) {
	s := "The quick brown fox jumps over the lazy dog"

	// encrypt
	var outbuf bytes.Buffer
	zipw := NewWriter(&outbuf)
	w, err := zipw.Encrypt("test.txt", "password", StandardEncryption)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.Copy(w, bytes.NewReader([]byte(s))); err != nil {
		t.Fatal(err)
	}

	zipw.Flush()
	zipw.Close()

	// decrypt
	zipr, err := NewReader(bytes.NewReader(outbuf.Bytes()), int64(outbuf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	f := zipr.File[0]
	f.SetPassword("password")

	r, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}

	var inbuf bytes.Buffer
	if _, err := io.Copy(&inbuf, r); err != nil {
		t.Fatal(err)
	}

	if inbuf.String() != s {
		t.Errorf(`expected %s, got %s`, s, inbuf.String())
	}
}
