package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config 配置
const (
	DefaultRequestCount = 5               // 每个IP请求次数
	Timeout             = 2 * time.Second // 单次请求超时时间
	ConcurrentWorkers   = 32              // 并发协程数
)

// Result 存储单个IP的测试结果
type Result struct {
	IP         string
	Latencies  []time.Duration
	AvgLatency time.Duration
	Success    int
	Errors     int
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	// 1. 获取输入
	fmt.Print("请输入IP列表文件路径 (默认 ips.txt): ")
	filePath, _ := reader.ReadString('\n')
	filePath = strings.TrimSpace(filePath)
	// 处理Windows换行符
	filePath = strings.ReplaceAll(filePath, "\r", "")
	if filePath == "" {
		filePath = "ips.txt"
	}

	fmt.Print("请输入测试URL (例如 https://www.example.com): ")
	targetURLStr, _ := reader.ReadString('\n')
	targetURLStr = strings.TrimSpace(targetURLStr)
	targetURLStr = strings.ReplaceAll(targetURLStr, "\r", "")

	if targetURLStr == "" {
		fmt.Println("URL不能为空")
		return
	}

	// 如果用户没有输入scheme，尝试自动补全，默认https
	if !strings.HasPrefix(targetURLStr, "http://") && !strings.HasPrefix(targetURLStr, "https://") {
		targetURLStr = "https://" + targetURLStr
	}

	// 解析URL
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		fmt.Printf("URL解析失败: %v\n", err)
		return
	}

	// 2. 读取IP文件
	ips, err := readIPs(filePath)
	if err != nil {
		fmt.Printf("读取文件失败: %v\n", err)
		return
	}
	fmt.Printf("读取到 %d 个IP地址，准备开始测试...\n", len(ips))

	// 3. 并发测试
	results := make([]Result, 0, len(ips))
	var mutex sync.Mutex
	var wg sync.WaitGroup

	jobs := make(chan string, len(ips))

	// 启动工作协程
	for i := 0; i < ConcurrentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				res := testIP(ip, targetURL)
				// 只有至少成功一次才记录
				if res.Success > 0 {
					mutex.Lock()
					results = append(results, res)
					mutex.Unlock()
					fmt.Printf("\r[已测试] %s 平均耗时: %v     ", ip, res.AvgLatency)
				}
			}
		}()
	}

	// 发送任务
	for _, ip := range ips {
		jobs <- ip
	}
	close(jobs)

	wg.Wait()
	fmt.Println("\n\n测试完成，正在整理结果...")

	// 4. 结果排序与输出
	sort.Slice(results, func(i, j int) bool {
		return results[i].AvgLatency < results[j].AvgLatency
	})

	fmt.Println("\n================ 测试结果 (Top 20) ================")
	fmt.Printf("%-40s | %-15s | %-10s\n", "IP地址", "平均耗时", "成功率")
	fmt.Println(strings.Repeat("-", 75))

	limit := 20
	if len(results) < limit {
		limit = len(results)
	}

	for i := 0; i < limit; i++ {
		res := results[i]
		successRate := float64(res.Success) / float64(DefaultRequestCount) * 100
		fmt.Printf("%-40s | %-15v | %.0f%%\n", res.IP, res.AvgLatency, successRate)
	}

	// 保存完整结果到文件
	saveResults(results)
}

func readIPs(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			// 简单验证是否是有效IP
			if net.ParseIP(line) != nil {
				ips = append(ips, line)
			}
		}
	}
	return ips, scanner.Err()
}

func testIP(ip string, targetURL *url.URL) Result {
	res := Result{IP: ip}

	// 确定端口
	port := targetURL.Port()
	if port == "" {
		if targetURL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	// 格式化地址，如果是IPv6需要加括号
	dialAddr := ip
	if strings.Contains(ip, ":") && !strings.HasPrefix(ip, "[") {
		dialAddr = "[" + ip + "]"
	}
	address := fmt.Sprintf("%s:%s", dialAddr, port)

	// 自定义Transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: Timeout,
			}
			// 强制连接到指定IP
			return d.DialContext(ctx, "tcp", address)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,                 // 忽略证书校验
			ServerName:         targetURL.Hostname(), // SNI必须是域名
		},
		DisableKeepAlives: true, // 禁用KeepAlive，每次新建连接测速更准确反映用户首次体验
		MaxIdleConns:      1,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   Timeout,
	}

	// 循环请求
	var durations []time.Duration

	for i := 0; i < DefaultRequestCount; i++ {
		start := time.Now()

		req, err := http.NewRequest("GET", targetURL.String(), nil)
		if err != nil {
			res.Errors++
			continue
		}

		// 确保Host头正确，并且设置User-Agent
		req.Host = targetURL.Hostname()
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) iptest/1.0")

		resp, err := client.Do(req)
		if err != nil {
			res.Errors++
			continue
		}
		resp.Body.Close()

		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		res.Success++

		// 简单的间隔，避免瞬间过高频
		time.Sleep(50 * time.Millisecond)
	}

	res.Latencies = durations
	res.AvgLatency = calculateAverage(durations)

	return res
}

func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// 排序
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// 如果数据量少，直接平均
	if len(durations) <= 2 {
		var sum time.Duration
		for _, d := range durations {
			sum += d
		}
		return sum / time.Duration(len(durations))
	}

	// 去除最大值和最小值 (异常数据)
	validData := sorted[1 : len(sorted)-1]

	var sum time.Duration
	for _, d := range validData {
		sum += d
	}
	return sum / time.Duration(len(validData))
}

func saveResults(results []Result) {
	f, err := os.Create("result.csv")
	if err != nil {
		fmt.Println("无法创建结果文件:", err)
		return
	}
	defer f.Close()

	// 添加BOM头，防止Excel打开中文乱码（虽然这里没有中文列内容，但习惯加上）
	f.WriteString("\xEF\xBB\xBF")
	f.WriteString("IP,平均耗时(ms),成功次数,失败次数\n")
	for _, r := range results {
		f.WriteString(fmt.Sprintf("%s,%d,%d,%d\n", r.IP, r.AvgLatency.Milliseconds(), r.Success, r.Errors))
	}
	fmt.Printf("完整结果已保存到 %s\n", f.Name())
}
