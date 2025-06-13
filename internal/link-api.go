package internal

import (
	"github.com/busy-cloud/boat/api"
	"github.com/busy-cloud/boat/curd"
	"github.com/gin-gonic/gin"
)

func init() {
	api.Register("GET", "tcp-server/link/list", curd.ApiListHook[Link](getLinksInfo))
	api.Register("POST", "tcp-server/link/create", curd.ApiCreate[Link]())
	api.Register("POST", "tcp-server/link/search", curd.ApiSearchHook[Link](getLinksInfo))
	api.Register("GET", "tcp-server/link/:id", curd.ApiGetHook[Link](getLinkInfo))

	api.Register("POST", "tcp-server/link/:id", curd.ApiUpdateHook[Link](nil, func(m *Link) error {
		_ = unloadLink(m.Id)
		return nil
	}, "id", "name", "disabled", "protocol", "protocol_options"))

	api.Register("GET", "tcp-server/link/:id/delete", curd.ApiDeleteHook[Link](nil, func(m *Link) error {
		_ = unloadLink(m.Id)
		return nil
	}))

	api.Register("GET", "tcp-server/link/:id/enable", curd.ApiDisable[Link](false))
	api.Register("GET", "tcp-server/link/:id/disable", curd.ApiDisableHook[Link](true, nil, func(id any) error {
		_ = unloadLink(id.(string))
		return nil
	}))

	api.Register("GET", "tcp-server/link/:id/close", linkClose)
}

func getLinksInfo(ds []*Link) error {
	for _, d := range ds {
		_ = getLinkInfo(d)
	}
	return nil
}

func getLinkInfo(d *Link) error {
	l := links.Load(d.Id)
	if l != nil {
		d.Status = l.Status
	}
	return nil
}

func unloadLink(id string) error {
	c := links.LoadAndDelete(id)
	if c != nil {
		return c.Close()
	}
	return nil
}

func linkClose(ctx *gin.Context) {
	err := unloadLink(ctx.Param("id"))
	if err != nil {
		api.Error(ctx, err)
		return
	}
	api.OK(ctx, nil)
}
