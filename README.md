# Go TCP Connection Pool

一个用于 TCP 长连接管理的轻量级连接池，可作为底层组件集成到 HTTP 代理或中间件中。

## 核心特性

- **并发安全**：使用 `sync.Mutex` 配合带缓冲的 `chan` 实现，通过 `go test -race` 检测。
- **依赖注入（DialFunc）**：通过传入自定义的 `dialFunc func() (net.Conn, error)` 解耦具体的网络拨号逻辑，使其能作为通用组件无缝接入各类网关或代理项目。
- **容量控制**：
  - `MaxIdle`：通道容量，控制最大空闲连接数。
  - `MaxActive`：全局最大连接数限制（活跃 + 空闲）。达到上限时，`Get` 请求会阻塞排队。
- **假死连接探测**：在获取连接时，进行 1 毫秒的读超时探测，防止复用已被对端或中间设备断开的连接。
- **惰性清理 (Lazy Cleanup)**：连接入池时记录时间戳，出池时校验是否超过 `idleTimeout`。避免了后台定时清理带来的全量遍历和锁竞争开销。
- **最小化锁粒度**：仅在修改 `totalConns` 状态时加锁，网络拨号 (`net.Dial`) 等耗时操作均在锁外进行。

## 核心 API

- `NewConnPool(maxIdle int, idleTimeout time.Duration) *ConnPool`：初始化连接池。
- `Get() (net.Conn, error)`：获取连接。池空则新建；达上限则阻塞等待；获取时执行过期与假死检测。
- `Put(conn net.Conn)`：归还连接。若空闲通道满则直接关闭该连接并释放名额。

## 测试

项目包含并发压测用例。运行以下命令：
```bash
go test -race -v
```
覆盖场景：
1. 并发竞争：10 个并发协程争抢 4 个连接上限，验证名额限制与阻塞唤醒逻辑。

2. 惰性清理：设置短超时，验证过期连接在出池时被正确销毁并触发新建。

## 后续优化

- 引入 Sync.Map 或分片锁，支持多目标地址（Multi-Host）的池化复用。

- 增加连接数监控指标（Metrics），方便集成 Prometheus。

- 引入连接池预热机制。