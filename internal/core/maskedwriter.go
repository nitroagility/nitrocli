package core

import "io"

// maskedWriter wraps an io.Writer and masks secrets in realtime.
// It forwards every write immediately, replacing secret values with asterisks.
type maskedWriter struct {
	dest   io.Writer
	masker *Masker
}

func newMaskedWriter(dest io.Writer, masker *Masker) *maskedWriter {
	return &maskedWriter{dest: dest, masker: masker}
}

func (w *maskedWriter) Write(p []byte) (int, error) {
	masked := w.masker.Mask(string(p))
	_, err := io.WriteString(w.dest, masked)
	return len(p), err
}
