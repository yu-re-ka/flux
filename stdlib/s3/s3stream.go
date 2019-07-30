package s3

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
	"math"
)

// Seek whence values
const (
	SeekStart   = 0 // seek relative to the origin of the file
	SeekCurrent = 1 // seek relative to the current offset
	SeekEnd     = 2 // seek relative to the end
)

type ReadWriteSeeker struct {
	// implements flux.S3ReadWriteSeeker or whatever
	akid      string
	secretKey string
	region    *string
	bucket    *string
	path      *string

	sess *session.Session

	base      int64
	off       int64
	limit     int64
	initLimit bool

	requestInput s3.GetObjectInput
	uploadInput  s3manager.UploadInput
}

func NewReadWriteSeeker(akid, secretKey string, region, bucket, path string) ReadWriteSeeker {
	return ReadWriteSeeker{
		akid:      akid,
		secretKey: secretKey,
		bucket:    aws.String(bucket),
		region:    aws.String(region),
		path:      aws.String(path),
	}
}

func (s *ReadWriteSeeker) Open() error {
	sess, err := session.NewSession(&aws.Config{
		Region:      s.region,
		Credentials: credentials.NewStaticCredentials(s.akid, s.secretKey, ""),
	})
	if err != nil {
		return err
	}
	s.sess = sess

	s.requestInput = s3.GetObjectInput{
		Bucket: s.bucket,
		Key:    s.path,
	}

	s.uploadInput = s3manager.UploadInput{
		Bucket: s.bucket,
		Key:    s.path,
	}

	// no need to close session, it's just a shared configuration

	return nil
}

func (s *ReadWriteSeeker) Read(p []byte) (int, error) {
	if !s.initLimit {
		i, err := s.getFileSize(s.requestInput)
		if err != nil {
			return 0, err
		}
		s.limit = i - 1

		s.initLimit = true
	}

	if s.off > s.limit {
		return 0, io.EOF
	}

	if s.off + int64(len(p)) > s.limit {
		p = p[:s.limit - s.off]
	}

	buf := aws.NewWriteAtBuffer(p)

	s.requestInput.Range = aws.String(fmt.Sprintf("bytes=%v-%v", s.off, s.off + int64(math.Min(float64(len(p)) - 1, float64(s.limit)))))
	fmt.Printf("Off:%v\nLimit: %v\nRange: %v", s.off, s.limit, *s.requestInput.Range)
	downloader := s3manager.NewDownloader(s.sess)

	numBytes, err := downloader.Download(buf, &s.requestInput)
	s.off += numBytes

	return int(numBytes), err
}

func (s *ReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	if !s.initLimit {
		i, err := s.getFileSize(s.requestInput)
		if err != nil {
			return 0, err
		}
		s.limit = i

		s.initLimit = true
	}

	switch whence {
	case SeekStart:
		offset += s.base
	case SeekCurrent:
		offset += s.off
	case SeekEnd:
		offset += s.limit
	}
	if offset < s.base {
		return 0, fmt.Errorf("cannot seek to an offset before the start of the file (fileSize = %v)", s.limit)
	}

	s.off = offset
	fmt.Printf("limit: %v, offset: %v", s.limit, s.off)
	return offset - s.base, nil
}

func (s *ReadWriteSeeker) Write(p []byte) (int, error) {
	return 0, nil
}

func (s *ReadWriteSeeker) getFileSize(input s3.GetObjectInput) (int64, error) {
	temp := &s3.HeadObjectInput{
		Bucket: s.bucket,
		Key:    s.path,
	}

	svc := s3.New(s.sess)
	out, err := svc.HeadObject(temp)
	if err != nil {
		return 0, err
	}
	return *out.ContentLength, err
}

func (s *ReadWriteSeeker) Write(p []byte) (int, error) {
	r := bytes.NewReader(p)

	uploader := s3manager.NewUploader(s.sess)

	s.uploadInput.Body = r

	_, err := uploader.Upload(&s.uploadInput)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}
