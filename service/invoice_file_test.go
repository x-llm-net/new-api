package service

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreInvoicePDF_PrivateRoundTrip(t *testing.T) {
	t.Setenv("INVOICE_STORAGE_PATH", t.TempDir())
	content := []byte("%PDF-1.7\ninvoice-test")
	key, err := StoreInvoicePDF(bytes.NewReader(content))
	require.NoError(t, err)
	require.Len(t, key, 32)

	path, err := InvoicePDFPath(key)
	require.NoError(t, err)
	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, stored)

	require.NoError(t, DeleteInvoicePDF(key))
	_, err = InvoicePDFPath(key)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStoreInvoicePDF_RejectsNonPDF(t *testing.T) {
	t.Setenv("INVOICE_STORAGE_PATH", t.TempDir())
	_, err := StoreInvoicePDF(bytes.NewBufferString("not a pdf"))
	assert.ErrorIs(t, err, ErrInvalidInvoicePDF)
}

func TestInvoicePDFPath_RejectsInvalidKey(t *testing.T) {
	t.Setenv("INVOICE_STORAGE_PATH", t.TempDir())
	_, err := InvoicePDFPath("../invoice")
	assert.ErrorIs(t, err, ErrInvalidInvoicePDF)
}

func TestStoreInvoicePDF_RejectsOversizedFile(t *testing.T) {
	t.Setenv("INVOICE_STORAGE_PATH", t.TempDir())
	content := append(
		[]byte("%PDF-"),
		bytes.Repeat([]byte("x"), int(MaxInvoiceFileBytes))...,
	)
	_, err := StoreInvoicePDF(bytes.NewReader(content))
	assert.ErrorIs(t, err, ErrInvoiceFileTooLarge)
}
