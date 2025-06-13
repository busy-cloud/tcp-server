package internal

import (
	"github.com/busy-cloud/boat/lib"
	"github.com/god-jason/iot-master/link"
	"net"
)

type Link struct {
	link.Link   `xorm:"extends"`
	link.Status `xorm:"-"`
	net.Conn    `xorm:"-"`
}

var links lib.Map[Link]
