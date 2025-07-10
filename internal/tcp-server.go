package internal

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/busy-cloud/boat/db"
	"github.com/busy-cloud/boat/mqtt"
	"github.com/god-jason/iot-master/link"
	"github.com/spf13/cast"
	"go.uber.org/multierr"
	"net"
	"regexp"
	"sync/atomic"
	"time"
)

func init() {
	db.Register(&TcpServer{})
}

type RegisterOptions struct {
	Type  string `json:"type,omitempty"`  //注册类型 string, json, hex
	Regex string `json:"regex,omitempty"` //ID正则表达式
	Field string `json:"field,omitempty"` //注册包为JSON时，取一个字段作为ID
}

type TcpServer struct {
	Id              string           `json:"id,omitempty" xorm:"pk"`
	Name            string           `json:"name,omitempty"`
	Description     string           `json:"description,omitempty"`
	Port            uint16           `json:"port,omitempty"`                            //端口号
	Multiple        bool             `json:"multiple,omitempty"`                        //多入（需要设置注册包）
	Register        bool             `json:"register,omitempty"`                        //启用注册包
	RegisterOptions *RegisterOptions `json:"register_options,omitempty" xorm:"json"`    //注册包参数
	Protocol        string           `json:"protocol,omitempty"`                        //通讯协议
	ProtocolOptions map[string]any   `json:"protocol_options,omitempty" xorm:"json"`    //通讯协议参数
	Disabled        bool             `json:"disabled,omitempty"`                        //禁用
	Created         time.Time        `json:"created,omitempty,omitzero" xorm:"created"` //创建时间

	link.Status `xorm:"-"`
}

type TcpServerImpl struct {
	*TcpServer

	buf    []byte
	opened bool

	listener net.Listener
	children map[string]net.Conn

	regex *regexp.Regexp

	increment atomic.Uint64
}

var idReg = regexp.MustCompile(`^\w{2,128}$`)

func NewTcpServer(l *TcpServer) *TcpServerImpl {
	server := &TcpServerImpl{
		TcpServer: l,
		buf:       make([]byte, 4096),
		children:  make(map[string]net.Conn),
	}
	if server.Register {
		if server.RegisterOptions != nil && server.RegisterOptions.Regex != "" {
			server.regex, _ = regexp.Compile("^" + server.RegisterOptions.Regex + "$")
		}
		if server.regex == nil {
			server.regex = idReg
		}
	}
	return server
}

func (s *TcpServerImpl) Open() (err error) {
	if s.opened {
		_ = s.Close()
	}

	//addr := fmt.Sprintf("%s:%d", s.Address, s.Port)
	addr := fmt.Sprintf(":%d", s.Port)
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		s.Error = err.Error()
		return
	}

	s.opened = true

	go s.accept()

	//topic := fmt.Sprintf("link/%s/open", s.Id)
	//mqtt.Publish(topic, nil)

	return
}

func (s *TcpServerImpl) Close() error {
	s.opened = false
	var err error

	//停止监听
	if s.listener != nil {
		err = multierr.Append(err, s.listener.Close())
		//s.listener = nil
	}

	//关闭子连接
	for _, conn := range s.children {
		err = multierr.Append(err, conn.Close())
	}
	s.children = make(map[string]net.Conn)

	return err
}

func (s *TcpServerImpl) receive(id string, reg []byte, conn net.Conn) {
	//从数据库中查询
	var l Link
	//xorm.ErrNotExist //db.Engine.Exist()
	//.Where("linker=", "tcp-server").And("id=", id)
	has, err := db.Engine().ID(id).Get(&l)
	if err != nil {
		_, _ = conn.Write([]byte(err.Error()))
		_ = conn.Close()
		return
	}
	//查不到
	if !has {
		l.Id = id
		l.Linker = "tcp-server"
		l.Protocol = s.Protocol //继承协议
		l.ProtocolOptions = s.ProtocolOptions
		_, err = db.Engine().InsertOne(&l)
		if err != nil {
			_, _ = conn.Write([]byte(err.Error()))
			_ = conn.Close()
			return
		}
	} else {
		if l.Disabled {
			_, _ = conn.Write([]byte("disabled"))
			_ = conn.Close()
			return
		}
	}

	//赋值连接和状态
	l.Conn = conn
	l.Running = true
	l.Error = ""

	s.children[id] = &l
	links.Store(id, &l)

	//连接
	topicOpen := fmt.Sprintf("link/tcp-server/%s/open", id)
	mqtt.Publish(topicOpen, reg)
	if l.Protocol != "" {
		topicOpen = fmt.Sprintf("protocol/%s/link/tcp-server/%s/open", l.Protocol, id)
		mqtt.Publish(topicOpen, l.ProtocolOptions)
	}

	topicUp := fmt.Sprintf("link/tcp-server/%s/up", id)
	topicUpProtocol := fmt.Sprintf("protocol/%s/link/tcp-server/%s/up", s.Protocol, id)

	var n int
	var e error
	buf := make([]byte, 4096)
	for {
		n, e = conn.Read(buf)
		if e != nil {
			_ = conn.Close()
			break
		}

		data := buf[:n]
		//转发
		mqtt.Publish(topicUp, data)
		if s.Protocol != "" {
			mqtt.Publish(topicUpProtocol, data)
		}
	}

	l.Conn = nil
	l.Running = false
	l.Error = e.Error()

	//下线
	topicClose := fmt.Sprintf("link/tcp-server/%s/close", id)
	mqtt.Publish(topicClose, e.Error())
	if s.Protocol != "" {
		topic := fmt.Sprintf("protocol/%s/link/tcp-server/%s/close", s.Protocol, id)
		mqtt.Publish(topic, e.Error())
	}

	delete(s.children, id)
	//links.Delete(id)
}

func (s *TcpServerImpl) accept() {
	s.Running = true

	for s.opened {
		conn, err := s.listener.Accept()
		if err != nil {
			s.Error = err.Error()
			break
		}

		//单例
		if !s.Multiple {
			s.receive(s.Id, nil, conn)
			continue
		}

		//未启用注册包
		if !s.Register {
			inc := s.increment.Add(1)
			id := fmt.Sprintf("%s.%d", s.Id, inc)
			s.receive(id, nil, conn)
			continue
		}

		//读超时??
		n, e := conn.Read(s.buf[:])
		if e != nil {
			//log.Error(e)
			_ = conn.Close()
			continue
		}
		data := s.buf[:n]
		id := string(data)

		if s.RegisterOptions == nil {
			s.receive(id, nil, conn)
			continue
		}

		//注册包类型
		switch s.RegisterOptions.Type {
		case "string":
		case "hex":
			id = hex.EncodeToString(data)
		case "json":
			var reg map[string]any
			err = json.Unmarshal(data, &reg)
			if err != nil {
				_, _ = conn.Write([]byte("require json pack"))
				_ = conn.Close()
				continue
			}

			//默认字段是id
			if v, ok := reg[s.RegisterOptions.Field]; ok {
				id = cast.ToString(v)
			} else {
				_, _ = conn.Write([]byte("require field " + s.RegisterOptions.Field))
				_ = conn.Close()
				continue
			}

			//取默认id
			if v, ok := reg["id"]; ok {
				id = cast.ToString(v)
			} else if v, ok = reg["sn"]; ok {
				id = cast.ToString(v)
			} else if v, ok = reg["key"]; ok {
				id = cast.ToString(v)
			} else {
				_, _ = conn.Write([]byte("require id field "))
				_ = conn.Close()
				continue
			}
		}

		//验证合法性
		if !s.regex.MatchString(id) {
			_, _ = conn.Write([]byte("invalid id"))
			_ = conn.Close()
			continue
		}

		//开始接收数据
		go s.receive(id, data, conn)
	}

	_ = s.listener.Close()
	s.listener = nil

	s.Running = false
}
