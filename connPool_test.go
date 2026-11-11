package main // 如果你的 package 是其他的，请修改这里

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// startMockServer 启动一个本地 TCP 服务，用来接收连接池的 Dial 请求
func startMockServer(t *testing.T) (net.Listener, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatalf("启动 Mock Server 失败: %v", err)
	}

	// 后台接受连接，不主动发数据，让客户端 Read 时触发 Timeout
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // Listener 关闭时退出
			}
			// 保持连接打开，直到客户端关闭
			go func(c net.Conn) {
				buf := make([]byte, 1)
				c.Read(buf) // 阻塞等待客户端关闭
				c.Close()
			}(conn)
		}
	}()

	return listener, func() {
		listener.Close()
	}
}

func TestConnPool_Concurrency(t *testing.T) {
	// 1. 启动 Mock Server
	_, cleanup := startMockServer(t)
	defer cleanup()

	// 2. 初始化连接池：maxIdle=2，则 maxActive=4
	pool := NewConnPool(2, 60*time.Second)

	// 3. 并发测试：10 个 goroutine 抢 4 个配额
	concurrency := 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()

			// 从池中获取连接
			conn, err := pool.Get()
			if err != nil {
				t.Errorf("G%d 获取连接失败: %v", id, err)
				return
			}
			fmt.Printf("G%d 成功获取连接\n", id)

			// 模拟业务执行时间
			time.Sleep(100 * time.Millisecond)

			// 归还连接
			pool.Put(conn)
			fmt.Printf("G%d 归还连接\n", id)
		}(i)
	}

	// 等待所有并发任务完成
	wg.Wait()
	fmt.Println("所有并发任务执行完毕")

	// 4. 验证池子的最终状态
	// 此时应该还剩下 2 个闲置连接在 chan 中，总连接数为 2
	if len(pool.idleConns) != 2 {
		t.Errorf("测试结束后 idleConns 数量预期为 2，实际为 %d", len(pool.idleConns))
	}
	if pool.totalConns != 2 {
		t.Errorf("测试结束后 totalConns 预期为 2，实际为 %d", pool.totalConns)
	}
}

// TestConnPool_LazyCleanup 懒清理测试
func TestConnPool_LazyCleanup(t *testing.T) {
	_, cleanup := startMockServer(t)
	defer cleanup()

	// 1. 设置极短的超时时间：100ms
	pool := NewConnPool(2, 100*time.Millisecond)

	// 2. 拿一个连接，马上还回去
	conn1, err := pool.Get()
	if err != nil {
		t.Fatalf("首次获取失败: %v", err)
	}
	pool.Put(conn1)

	if pool.totalConns != 1 {
		t.Errorf("预期 totalConns 为 1，实际为 %d", pool.totalConns)
	}

	// 3. 睡眠 150ms，等待连接过期
	fmt.Println("等待 150ms 让连接过期...")
	time.Sleep(150 * time.Millisecond)

	// 4. 再次获取，期望：懒清理机制生效，丢弃旧连接并新建
	conn2, err := pool.Get()
	if err != nil {
		t.Fatalf("二次获取失败: %v", err)
	}
	fmt.Println("二次获取成功，说明过期连接被清理")

	// 5. 验证状态：过期连接被销毁，新建了一个，所以总数还是 1，通道空闲为 0
	if pool.totalConns != 1 {
		t.Errorf("预期 totalConns 为 1，实际为 %d", pool.totalConns)
	}
	if len(pool.idleConns) != 0 {
		t.Errorf("预期 idleConns 为 0，实际为 %d", len(pool.idleConns))
	}

	pool.Put(conn2)
}
