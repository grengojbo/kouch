package io

import (
	"io"

	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kouch/internal/errors"
)

type exitStatusWriter struct {
	io.WriteCloser
}

var _ io.WriteCloser = &exitStatusWriter{}
var _ WrappedWriter = &exitStatusWriter{}

func (w *exitStatusWriter) Underlying() io.Writer {
	return w.WriteCloser
}

func (w *exitStatusWriter) Write(p []byte) (int, error) {
	c, err := w.WriteCloser.Write(p)
	return c, errors.WrapExitError(chttp.ExitWriteError, err)
}

type exitStatusReadCloser struct {
	io.ReadCloser
}

var _ io.ReadCloser = &exitStatusReadCloser{}

func (r *exitStatusReadCloser) Read(p []byte) (int, error) {
	c, err := r.ReadCloser.Read(p)
	if err == nil || err == io.EOF {
		return c, err
	}
	return c, &errors.ExitError{Err: err, ExitCode: chttp.ExitReadError}
}

type nopCloser struct {
	io.Writer
}

var _ io.WriteCloser = &nopCloser{}

func (w *nopCloser) Close() error { return nil }
