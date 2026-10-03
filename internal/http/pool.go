package http

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

var jsonBufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// GetBuffer acquires a recycled buffer from sync.Pool.
func GetBuffer() *bytes.Buffer {
	buf := jsonBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuffer returns a buffer to the sync.Pool if within memory limits.
func PutBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 64*1024 { // Drop oversized buffers to avoid memory retention
		return
	}
	jsonBufferPool.Put(buf)
}

// EncodeJSONPooled serializes data using a pooled bytes.Buffer.
func EncodeJSONPooled(w io.Writer, data any) error {
	buf := GetBuffer()
	defer PutBuffer(buf)

	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}
