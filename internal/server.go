package internal

import (
	"fmt"
	"github.com/busy-cloud/boat/db"
	"github.com/busy-cloud/boat/lib"
	"github.com/busy-cloud/boat/log"
)

var servers lib.Map[TcpServerImpl]

func StartServers() {
	//加载连接器
	var servers []*TcpServer
	err := db.Engine().Find(&servers)
	if err != nil {
		log.Error(err)
		return
	}
	for _, server := range servers {
		if server.Disabled {
			log.Info("server %s is disabled", server.Id)
			continue
		}
		err := FromServer(server)
		if err != nil {
			log.Error(err)
		}
	}
}

func StopServers() {
	servers.Range(func(name string, server *TcpServerImpl) bool {
		_ = server.Close()
		return true
	})
}

func FromServer(m *TcpServer) error {
	server := NewTcpServer(m)

	//保存
	val := servers.LoadAndStore(server.Id, server)
	if val != nil {
		err := val.Close()
		if err != nil {
			log.Error(err)
		}
	}

	//启动
	err := server.Open()
	if err != nil {
		return err
	}

	return nil
}

func LoadServer(id string) error {
	var l TcpServer
	has, err := db.Engine().ID(id).Get(&l)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("tcp server %s not found", id)
	}

	return FromServer(&l)
}

func UnloadServer(id string) error {
	val := servers.LoadAndDelete(id)
	if val != nil {
		return val.Close()
	}
	return nil
}
