package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type ConnPool struct {
	idleConns  chan net.Conn
	totalConns int32
	maxActive  int32
	mu         sync.Mutex
}

func NewConnPool(maxIdle int) *ConnPool {
	return &ConnPool{
		idleConns:  make(chan net.Conn, maxIdle),
		totalConns: 0,
		maxActive:  int32(2 * maxIdle),
	}
}

func (p *ConnPool) Get() (net.Conn, error) {
	for {
		select {
		case conn := <-p.idleConns:
			if p.isAlive(conn) {
				return conn, nil
			}
			conn.Close()
			p.mu.Lock()
			p.totalConns--
			p.mu.Unlock()
			continue
		default:
			fmt.Println("池子空了，新建连接")
			p.mu.Lock()
			if p.totalConns >= p.maxActive {
				p.mu.Unlock()
			} else {
				p.totalConns++
				p.mu.Unlock()

				conn, err := net.Dial("tcp", "127.0.0.1:8080")
				if err != nil {
					p.mu.Lock()
					p.totalConns--
					p.mu.Unlock()
					return nil, err
				}
				return conn, nil
			}
		}

		conn := <-p.idleConns

		if p.isAlive(conn) {
			return conn, nil
		}
		conn.Close()
		p.mu.Lock()
		p.totalConns--
		p.mu.Unlock()
	}
}

func (p *ConnPool) isAlive(conn net.Conn) bool {
	conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	var buf [1]byte
	_, err := conn.Read(buf[:])
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			conn.SetReadDeadline(time.Time{})
			return true
		}
	}
	return false
}

func (p *ConnPool) Put(conn net.Conn) {
	if conn == nil {
		return
	}
	select {
	case p.idleConns <- conn:
		// 塞进去了，安全归还
		fmt.Println("连接归还成功")
	default:
		// 通道满了，说明空闲连接太多，直接销毁
		conn.Close()
		p.mu.Lock()
		p.totalConns--
		p.mu.Unlock()
		fmt.Println("池子满了，多余连接已销毁")
	}
}
