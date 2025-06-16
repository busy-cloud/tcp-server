package internal

import (
	"github.com/busy-cloud/boat/api"
	"github.com/busy-cloud/boat/curd"
	"github.com/gin-gonic/gin"
)

func init() {
	api.Register("GET", "tcp-server/server/list", curd.ApiListHook[TcpServer](getServersInfo))
	api.Register("POST", "tcp-server/server/search", curd.ApiSearchHook[TcpServer](getServersInfo))
	api.Register("POST", "tcp-server/server/create", curd.ApiCreateHook[TcpServer](nil, func(m *TcpServer) error {
		_ = FromServer(m)
		return nil
	}))
	api.Register("GET", "tcp-server/server/:id", curd.ApiGetHook[TcpServer](getServerInfo))

	api.Register("POST", "tcp-server/server/:id", curd.ApiUpdateHook[TcpServer](nil, func(m *TcpServer) error {
		_ = FromServer(m)
		return nil
	}, "id", "name", "type", "port", "multiple", "register_options", "disabled", "protocol", "protocol_options"))

	api.Register("GET", "tcp-server/server/:id/delete", curd.ApiDeleteHook[TcpServer](nil, func(m *TcpServer) error {
		_ = UnloadServer(m.Id)
		return nil
	}))

	api.Register("GET", "tcp-server/server/:id/enable", curd.ApiDisableHook[TcpServer](false, nil, func(id any) error {
		_ = LoadServer(id.(string))
		return nil
	}))

	api.Register("GET", "tcp-server/server/:id/disable", curd.ApiDisableHook[TcpServer](true, nil, func(id any) error {
		_ = UnloadServer(id.(string))
		return nil
	}))

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
