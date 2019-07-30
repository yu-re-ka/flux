package s3

// Seek whence values
const (
	SeekStart   = 0 // seek relative to the origin of the file
	SeekCurrent = 1 // seek relative to the current offset
	SeekEnd     = 2 // seek relative to the end
)

type Stream struct {
	// implements flux.Stream or whatever
}

func (s *Stream) Open() {

}

func (s *Stream) Read() {

}

func (s *Stream) Seek(offset int64, whence int) (int64, error) {

}