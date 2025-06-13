package internal

import (
	"github.com/busy-cloud/boat/lib"
	"github.com/god-jason/iot-master/link"
	"net"
)

//type Link interface {
//	io.ReadWriteCloser
//	Open() error
//	Opened() bool
//	Connected() bool
//	Error() string
//}

type Link struct {
	link.Link //`xorm:"extends"`
	link.Status
	net.Conn
}

var links lib.Map[Link]
