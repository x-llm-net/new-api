package service

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxInvoiceFileBytes int64 = 10 * 1024 * 1024

var (
	ErrInvalidInvoicePDF   = errors.New("invalid invoice PDF")
	ErrInvoiceFileTooLarge = errors.New("invoice PDF exceeds 10 MB")
)

func invoiceStorageRoot() string {
	if configured := strings.TrimSpace(os.Getenv("INVOICE_STORAGE_PATH")); configured != "" {
		return configured
	}
	return filepath.Join("data", "invoices")
}

func StoreInvoicePDF(reader io.Reader) (string, error) {
	buffered := bufio.NewReader(reader)
	header, err := buffered.Peek(5)
	if err != nil || string(header) != "%PDF-" {
		return "", ErrInvalidInvoicePDF
	}

	root := invoiceStorageRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	key := hex.EncodeToString(randomBytes)
	temporary, err := os.CreateTemp(root, ".invoice-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	written, copyErr := io.Copy(temporary, io.LimitReader(buffered, MaxInvoiceFileBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written > MaxInvoiceFileBytes {
		return "", ErrInvoiceFileTooLarge
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, filepath.Join(root, key+".pdf")); err != nil {
		return "", err
	}
	return key, nil
}

func InvoicePDFPath(key string) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidInvoicePDF
	}
	for _, char := range key {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return "", ErrInvalidInvoicePDF
		}
	}
	path := filepath.Join(invoiceStorageRoot(), key+".pdf")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func DeleteInvoicePDF(key string) error {
	path, err := InvoicePDFPath(key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.Remove(path)
}
