package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/busy-cloud/boat/db"
	"github.com/busy-cloud/boat/mqtt"
	"github.com/god-jason/iot-master/link"
	"go.uber.org/multierr"
	"net"
	"regexp"
	"time"
)

func init() {
	db.Register(&TcpServer{})
}

type RegisterOptions struct {
	Type   string `json:"type,omitempty"`   //注册类型 string, json
	Regex  string `json:"regex,omitempty"`  //ID正则表达式
	Field  string `json:"field,omitempty"`  //注册包为JSON时，取一个字段作为ID
	Offset uint16 `json:"offset,omitempty"` //偏移，用于处理固定包头
	Length uint16 `json:"length,omitempty"` //取长度
}

type TcpServer struct {
	Id              string           `json:"id,omitempty" xorm:"pk"`
	Name            string           `json:"name,omitempty"`
	Description     string           `json:"description,omitempty"`
	Port            uint16           `json:"port,omitempty"`                            //端口号
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
}

var idReg = regexp.MustCompile(`^\w{2,128}$`)

func NewTcpServerMultiple(l *TcpServer) *TcpServerImpl {
	server := &TcpServerImpl{
		TcpServer: l,
		buf:       make([]byte, 4096),
		children:  make(map[string]net.Conn),
	}
	if server.RegisterOptions != nil && server.RegisterOptions.Regex != "" {
		server.regex, _ = regexp.Compile("^" + server.RegisterOptions.Regex + "$")
	}
	if server.regex == nil {
		server.regex = idReg
	}
	return server
}

func (s *TcpServerImpl) Read(p []byte) (n int, err error) {
	return 0, errors.New("unsupported read")
}

func (s *TcpServerImpl) Write(p []byte) (n int, err error) {
	return 0, errors.New("unsupported write")
}

func (s *TcpServerImpl) Opened() bool {
	return s.opened
}

func (s *TcpServerImpl) Connected() bool {
	return s.listener != nil
}

func (s *TcpServerImpl) Error() string {
	return s.TcpServer.Error
}

func (s *TcpServerImpl) Open() (err error) {
	defer func() {
		if err != nil {
			s.TcpServer.Error = err.Error()
		} else {
			s.TcpServer.Error = ""
		}
	}()

	if s.opened {
		_ = s.Close()
	}

	//addr := fmt.Sprintf("%s:%d", s.Address, s.Port)
	addr := fmt.Sprintf("%s:%d", "", s.Port)
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}

	s.opened = true

	go s.accept()

	topic := fmt.Sprintf("link/%s/open", s.Id)
	mqtt.Publish(topic, nil)

	return
}

func (s *TcpServerImpl) Close() error {
	s.opened = false
	var err error
	for _, conn := range s.children {
		err = multierr.Append(err, conn.Close())
	}
	s.children = make(map[string]net.Conn)
	if s.listener != nil {
		err = multierr.Append(err, s.listener.Close())
		s.listener = nil
	}
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

	//赋值连接
	l.Conn = conn

	s.children[id] = &l
	links.Store(id, &l)

	//连接
	topicOpen := fmt.Sprintf("link/tcp-server/%s/open", id)
	mqtt.Publish(topicOpen, reg)
	if l.Protocol != "" {
		topicOpen = fmt.Sprintf("protocol/%s/tcp-server/%s/open", l.Protocol, id)
		mqtt.Publish(topicOpen, l.ProtocolOptions)
	}

	topicUp := fmt.Sprintf("link/tcp-server/%s/up", id)
	topicUpProtocol := fmt.Sprintf("protocol/%s/tcp-server/%s/up", s.Protocol, id)

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

	//下线
	topicClose := fmt.Sprintf("link/tcp-server/%s/close", id)
	mqtt.Publish(topicClose, e.Error())
	if s.Protocol != "" {
		topic := fmt.Sprintf("protocol/%s/tcp-server/%s/close", s.Protocol, id)
		mqtt.Publish(topic, e.Error())
	}

	delete(s.children, id)
	links.Delete(id)
}

func (s *TcpServerImpl) accept() {
	for s.opened {
		conn, err := s.listener.Accept()
		if err != nil {
			break
		}

		//TODO 读超时
		n, e := conn.Read(s.buf[:])
		if e != nil {
			//log.Error(e)
			_ = conn.Close()
			continue
		}
		data := s.buf[:n]

		if s.RegisterOptions != nil {
			//去头
			if s.RegisterOptions.Offset > 0 {
				if int(s.RegisterOptions.Offset) > len(data) {
					_, _ = conn.Write([]byte("id too small"))
					_ = conn.Close()
					continue
				}
				data = data[s.RegisterOptions.Offset:]
			}
			//取定长
			if s.RegisterOptions.Length > 0 {
				if int(s.RegisterOptions.Length) > len(data) {
					_, _ = conn.Write([]byte("id too small"))
					_ = conn.Close()
					continue
				}
				data = data[:s.RegisterOptions.Length]
			}
		}

		id := string(data)

		//处理json包
		if s.RegisterOptions != nil && s.RegisterOptions.Type == "json" {
			var reg map[string]any
			err = json.Unmarshal(data, &reg)
			if err != nil {
				_, _ = conn.Write([]byte(err.Error()))
				_ = conn.Close()
				continue
			}

			var ok bool
			id, ok = reg[s.RegisterOptions.Field].(string)
			if !ok {
				_, _ = conn.Write([]byte("require field " + s.RegisterOptions.Field))
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
}
