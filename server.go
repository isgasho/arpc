// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

// Server definition
type Server struct {
	Accepted int64
	CurrLoad int64
	MaxLoad  int64

	Codec   Codec
	Handler Handler

	Listener net.Listener

	running bool
	chStop  chan error
}

func (s *Server) addLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, 1)
}

func (s *Server) subLoad() int64 {
	return atomic.AddInt64(&s.CurrLoad, -1)
}

func (s *Server) runLoop() error {
	var (
		err  error
		cli  *Client
		conn net.Conn
	)

	s.running = true
	defer close(s.chStop)

	for s.running {
		conn, err = s.Listener.Accept()
		if err == nil {
			load := s.addLoad()
			if s.MaxLoad <= 0 || load <= s.MaxLoad {
				s.Accepted++
				cli = newClientWithConn(conn, s.Codec, s.Handler, s.subLoad)
				s.Handler.OnConnected(cli)
				if _, ok := conn.(WebsocketConn); !ok {
					cli.Run()
				} else {
					cli.RunWebsocket()
				}
			} else {
				conn.Close()
				s.subLoad()
			}
		} else {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				logError("%v Accept error: %v; retrying...", s.Handler.LogTag(), err)
				time.Sleep(time.Second / 20)
			} else {
				logError("%v Accept error: %v", s.Handler.LogTag(), err)
				break
			}
		}
	}

	return err
}

// Serve starts rpc service with listener
func (s *Server) Serve(ln net.Listener) error {
	s.Listener = ln
	s.chStop = make(chan error)
	logInfo("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	defer logInfo("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Run starts a tcp service on addr
func (s *Server) Run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logInfo("%v Running failed: %v", s.Handler.LogTag(), err)
		return err
	}
	s.Listener = ln
	s.chStop = make(chan error)
	logInfo("%v Running On: \"%v\"", s.Handler.LogTag(), ln.Addr())
	// defer logInfo("%v Stopped", s.Handler.LogTag())
	return s.runLoop()
}

// Stop rpc service
func (s *Server) Stop() error {
	// logInfo("%v %v Stop...", s.Handler.LogTag(), s.Listener.Addr())
	defer logInfo("%v %v Stop", s.Handler.LogTag(), s.Listener.Addr())
	s.running = false
	s.Listener.Close()
	select {
	case <-s.chStop:
	case <-time.After(time.Second):
		return ErrTimeout
	default:
	}
	return nil
}

// Shutdown stop rpc service
func (s *Server) Shutdown(ctx context.Context) error {
	// logInfo("%v %v Shutdown...", s.Handler.LogTag(), s.Listener.Addr())
	defer logInfo("%v %v Shutdown", s.Handler.LogTag(), s.Listener.Addr())
	s.running = false
	s.Listener.Close()
	select {
	case <-s.chStop:
	case <-ctx.Done():
		return ErrTimeout
	}
	return nil
}

// NewServer factory
func NewServer() *Server {
	h := DefaultHandler.Clone()
	h.SetLogTag("[ARPC SVR]")
	return &Server{
		Codec:   DefaultCodec,
		Handler: h,
	}
}
