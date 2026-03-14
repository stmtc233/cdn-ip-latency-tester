# CDN IP Latency Tester

一个基于 Go 编写的命令行工具，用于对一组候选 IP 地址进行并发延迟测试，并按平均响应耗时排序输出结果。它会在请求时**强制连接指定 IP**，同时保留目标域名的 `Host` 与 TLS `SNI`，适合用于 CDN / 边缘节点 / 回源候选 IP 的可用性与速度筛选。

我建议你在 GitHub 上使用仓库名：`cdn-ip-latency-tester`

## 功能特点

- 支持从 [`ips.txt`](ips.txt) 批量读取 IP 列表
- 支持 IPv4 / IPv6
- 自动补全 URL 协议，未填写时默认补为 `https://`
- 并发测速，默认 32 个工作协程
- 每个 IP 默认测试 5 次，自动去掉最大值和最小值后计算平均值
- 请求时保留目标域名的 `Host` 和 TLS `SNI`
- 测试完成后输出 Top 20 结果
- 自动导出完整结果到 `result.csv`

## 适用场景

- 为某个域名筛选更快的 CDN 节点 IP
- 对可疑或候选边缘节点做快速连通性验证
- 批量比较不同 IP 对同一目标站点的首连响应速度
- 为自建网络脚本提供基础测速数据

## 项目结构

- [`main.go`](main.go)：主程序入口与测速逻辑
- [`ips.txt`](ips.txt)：待测试的 IP 列表样例 / 数据文件
- [`by.bat`](by.bat)：简单的 Linux AMD64 交叉编译脚本
- [`go.mod`](go.mod)：Go 模块定义

## 工作原理

程序核心逻辑位于 [`testIP()`](main.go:156)：

1. 读取用户输入的目标 URL
2. 根据 URL 自动确定端口（HTTPS 默认 443，HTTP 默认 80）
3. 使用自定义 [`http.Transport`](main.go:177) 覆盖拨号过程
4. 忽略原始解析地址，直接连接指定 IP
5. 请求时保留 [`req.Host`](main.go:211) 和 [`tls.Config.ServerName`](main.go:185)
6. 记录多次请求耗时并计算平均值
7. 将可用结果按平均延迟升序排序

这意味着它不仅是在“ping IP”，而是在模拟真实 HTTP/HTTPS 访问条件下测试某个 IP 对指定站点的访问体验。

## 环境要求

- Go 1.25+
- Windows / Linux / macOS 均可运行

## 快速开始

### 1. 准备 IP 列表

在 [`ips.txt`](ips.txt) 中每行填写一个 IP，示例：

```text
2408:873c:6810:3::14
2408:873c:6810:3::12
1.1.1.1
8.8.8.8
```

说明：

- 空行会被忽略
- 以 `#` 开头的行会被视为注释
- 无效 IP 会被自动跳过，具体见 [`readIPs()`](main.go:135)

### 2. 运行程序

```bash
go run main.go
```

程序会提示你输入：

1. IP 列表文件路径（默认 `ips.txt`）
2. 测试 URL，例如：

```text
https://www.example.com
```

如果你只输入域名，例如：

```text
www.example.com
```

程序会自动补全为 `https://www.example.com`，对应逻辑见 [`main()`](main.go:34)。

## 编译

### 本地编译

```bash
go build -o cdn-ip-latency-tester.exe .
```

### 使用批处理脚本交叉编译 Linux AMD64

```bat
by.bat
```

该脚本内容见 [`by.bat`](by.bat)。

## 输出结果

程序结束后会输出：

- 控制台 Top 20 最快 IP
- 完整测速结果文件 `result.csv`

`result.csv` 包含以下字段：

- `IP`
- `平均耗时(ms)`
- `成功次数`
- `失败次数`

CSV 导出逻辑见 [`saveResults()`](main.go:266)。

## 默认参数

默认配置定义在 [`main.go`](main.go) 的常量区：

- [`DefaultRequestCount`](main.go:20)：每个 IP 测试 5 次
- [`Timeout`](main.go:21)：单次请求超时 2 秒
- [`ConcurrentWorkers`](main.go:22)：并发协程数 32

如果你后续准备继续完善项目，优先建议把这些参数改成命令行参数，以方便自动化使用。

## 注意事项

- 当前 TLS 校验已关闭，见 [`InsecureSkipVerify`](main.go:186)，这是为了方便直接对候选 IP 做测速，但**不适合用于严格安全校验场景**
- 当前排序只保留“至少成功一次”的 IP，逻辑见 [`main.go`](main.go:91)
- 这是一个测速工具，不保证目标 IP 一定适合生产环境长期使用
- 若目标站点有强校验、WAF、限速或地域限制，测速结果可能受影响


## License


