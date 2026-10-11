// SPDX-License-Identifier: AGPL-3.0-only

package collect

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/connectors/applehealth"
	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

func TestParseHeaders(t *testing.T) {
	hd := http.Header{}
	hd.Set(HeaderType, "HKQuantityTypeIdentifierHeartRate")
	hd.Set(HeaderDay, "2026-01-10")
	hd.Set(HeaderBatch, "0b7f-batch_1")
	hd.Set(HeaderPart, "12")
	q, msg := parseHeaders(hd)
	require.Empty(t, msg)
	assert.Equal(t, "HKQuantityTypeIdentifierHeartRate", q.dataType)
	assert.Equal(t, "2026-01-10", q.day.Format("2006-01-02"))
	assert.Equal(t, "0b7f-batch_1", q.batch)
	assert.Equal(t, 12, q.part)

	hd.Set(HeaderPart, "007")
	_, msg = parseHeaders(hd)
	assert.Contains(t, msg, HeaderPart, "one spelling per part number, so one key per part")

	hd.Set(HeaderPart, "1000001")
	_, msg = parseHeaders(hd)
	assert.Contains(t, msg, HeaderPart)

	hd.Set(HeaderPart, "0")
	hd.Set(HeaderBatch, strings.Repeat("b", 129))
	_, msg = parseHeaders(hd)
	assert.Contains(t, msg, HeaderBatch)
}

func gzipOf(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(b)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestGuardedReaderStopsAtTheCap(t *testing.T) {
	data := bytes.Repeat([]byte("{}\n"), 100) // 300 bytes
	zr, err := gzip.NewReader(bytes.NewReader(gzipOf(t, data)))
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, &guardedReader{r: zr, left: 299})
	assert.ErrorIs(t, err, errTooLarge)

	zr, err = gzip.NewReader(bytes.NewReader(gzipOf(t, data)))
	require.NoError(t, err)
	n, err := io.Copy(io.Discard, &guardedReader{r: zr, left: 300})
	assert.NoError(t, err, "exactly at the cap is fine")
	assert.Equal(t, int64(300), n)
}

func TestGuardedReaderMarksStreamErrors(t *testing.T) {
	full := gzipOf(t, bytes.Repeat([]byte("{\"a\":1}\n"), 1000))
	zr, err := gzip.NewReader(bytes.NewReader(full[:len(full)/2]))
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, &guardedReader{r: zr, left: 1 << 20})
	var se *streamError
	assert.True(t, errors.As(err, &se), "a truncated stream is a stream error: %v", err)
}

func TestValidateClassifies(t *testing.T) {
	h := &uploadHandler{d: UploadDeps{Device: applehealth.New()}, provider: applehealth.Slug}
	full := gzipOf(t, []byte("{\"a\":1}\n{\"b\":2}\n"))

	status, _ := h.validate(metaZero, full)
	assert.Equal(t, 0, status)

	status, msg := h.validate(metaZero, full[:len(full)-3])
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, msg, "gzip")

	status, _ = h.validate(metaZero, []byte("plain"))
	assert.Equal(t, http.StatusBadRequest, status)

	status, msg = h.validate(metaZero, gzipOf(t, []byte("{\"a\":1}\nnope\n")))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, msg, "line 2")
}

var metaZero = connector.UploadMeta{}
