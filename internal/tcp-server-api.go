package internal

import (
	"github.com/busy-cloud/boat/api"
	"github.com/gin-gonic/gin"
)

func init() {

	api.Register("GET", "tcp-server/server/:id/open", serverOpen)
	api.Register("GET", "tcp-server/server/:id/close", serverClose)

	api.Register("GET", "tcp-server/server/:id/status", serverStatus)
}

func getServersInfo(ds []*TcpServer) error {
	for _, d := range ds {
		_ = getServerInfo(d)
	}
	return nil
}

func getServerInfo(d *TcpServer) error {
	l := servers.Load(d.Id)
	if l != nil {
		d.Status = l.Status
	}
	return nil
}

func serverClose(ctx *gin.Context) {
	l := servers.Load(ctx.Param("id"))
	if l == nil {
		api.Fail(ctx, "找不到服务器")
		return
	}

	err := l.Close()
	if err != nil {
		api.Error(ctx, err)
		return
	}

	api.OK(ctx, nil)
}

func serverOpen(ctx *gin.Context) {
	l := servers.Load(ctx.Param("id"))
	if l != nil {
		_ = l.Close()
	}

	err := LoadServer(ctx.Param("id"))
	if err != nil {
		api.Error(ctx, err)
		return
	}

	api.OK(ctx, nil)
}

func serverStatus(ctx *gin.Context) {
	l := servers.Load(ctx.Param("id"))
	if l == nil {
		api.Fail(ctx, "找不到服务器")
		return
	}

	api.OK(ctx, l.Status)
}
