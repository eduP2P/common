package conn

import (
	"io"
	"time"
)

type MetaConn interface {
	io.Closer
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}
