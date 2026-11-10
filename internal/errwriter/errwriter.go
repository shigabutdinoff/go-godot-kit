// Package errwriter provides an io.Writer that remembers the first write error,
// so a series of writes can be checked once at the end.
package errwriter

import "io"

// Writer writes to an underlying io.Writer and keeps the first write error.
// It is not safe for concurrent use.
type Writer struct {
	w   io.Writer
	err error
}

var _ io.Writer = (*Writer)(nil)

// New returns a Writer that writes to w.
func New(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write writes p to the underlying writer and returns its result, keeping the first error.
func (w *Writer) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.Record(0, err)
	return n, err
}

// Record keeps err if no error was seen yet. It ignores the byte count,
// so the result of fmt.Fprint* can be passed as is.
func (w *Writer) Record(_ int, err error) {
	if w.err == nil {
		w.err = err
	}
}

// Err returns the first write error, or nil.
func (w *Writer) Err() error {
	return w.err
}
