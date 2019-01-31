package langserver

import (
	"io"
	"io/ioutil"
)

func ReadWriter(r io.Reader, w io.Writer) io.ReadWriteCloser {
	rc, ok := r.(io.ReadCloser)
	if !ok {
		rc = ioutil.NopCloser(r)
	}
	return &readWriteCloser{
		Reader: rc,
		Writer: w,
	}
}

type readWriteCloser struct {
	Reader io.ReadCloser
	Writer io.Writer
}

func (rwc *readWriteCloser) Close() error {
	if wc, ok := rwc.Writer.(io.WriteCloser); ok {
		if err := wc.Close(); err != nil {
			return err
		}
	}
	return rwc.Reader.Close()
}

func (rwc *readWriteCloser) Read(p []byte) (n int, err error) {
	return rwc.Reader.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (n int, err error) {
	return rwc.Writer.Write(p)
}
