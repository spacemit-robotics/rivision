// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rivision/rivision-cli/internal/camera"
	"github.com/rivision/rivision-cli/internal/embed"
	"github.com/rivision/rivision-cli/internal/owl"
	"github.com/rivision/rivision-cli/internal/semantic"
	"github.com/rivision/rivision-cli/internal/ui"
	"github.com/rivision/rivision-cli/internal/webserver"
	"github.com/rivision/rivision-cli/internal/webserver/websocket"
	"github.com/rivision/rivision-cli/internal/yolo"
	"github.com/spf13/cobra"
)

var (
	webuiPort       int
	go2rtcPort      int
	withRtspServer  bool
	withGo2rtc      bool
	noBrowser       bool
	mp4Dir          string

	// 新增参数
	configFile     string
	enableYolo     bool
	yoloModel      string
	vlmInterval    int
	vlmTriggerMode string

	// 分布式YOLO参数
	yoloMode string // local, distributed, auto

	// OWL (GB28181) 参数
	owlURL      string
	owlUsername string
	owlPassword string
	owlRSAAuth  bool
)

var webuiCmd = &cobra.Command{
	Use:   "webui",
	Short: "启动集成Web界面",
	Long: `启动集成Web界面 (go2rtc-vue + 推理分析)

OWL (GB28181) 集成:
  默认连接本地 OWL 服务 (http://127.0.0.1:15123)
  可通过参数自定义 OWL 连接配置

示例:
  # 使用默认配置启动
  rivision-cli webui

  # 指定 OWL 服务地址
  rivision-cli webui --owl-url=http://192.168.1.100:15123

  # 自定义 OWL 认证
  rivision-cli webui --owl-url=http://owl.local:15123 --owl-user=admin --owl-pass=secret

  # 禁用 OWL 集成 (仅使用 go2rtc)
  rivision-cli webui --owl-url=""

  # 指定端口和启用 YOLO
  rivision-cli webui --port=8080 --enable-yolo --yolo-model=./models/yolov8n.onnx`,
	RunE: func(cmd *cobra.Command, args []string) error {
		term := ui.NewTerminal()
		term.PrintHeader("RiVision CLI", "集成Web界面")

		// 获取可执行文件所在目录
		execPath, err := os.Executable()
		if err != nil {
			return err
		}
		// baseDir = 可执行文件的父目录（即 rivision-cli 目录，非 bin/ 子目录）
		// 例: execPath = /opt/rivision-cli/bin/rivision-cli → baseDir = /opt/rivision-cli
		baseDir := filepath.Dir(filepath.Dir(execPath))

		// 获取当前工作目录
		cwd, _ := os.Getwd()

		// 解析 mp4Dir 为绝对路径
		if !filepath.IsAbs(mp4Dir) {
			mp4Dir, _ = filepath.Abs(mp4Dir)
		}

		var processes []*exec.Cmd

		// 设置信号处理
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// 二进制提取目录：优先使用 cwd/bin，其次 baseDir/bin
		binDir := filepath.Join(cwd, "bin")
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			binDir = filepath.Join(baseDir, "bin")
		}
		os.MkdirAll(binDir, 0755)
		term.PrintInfo(fmt.Sprintf("📂 二进制目录: %s", binDir))

		// 1. 启动RTSP服务器（可选）
		if withRtspServer {
			var rtspExe string
			var rtspWorkDir string

			// 诊断：列出所有嵌入的二进制
			expectedName := embed.GetRtspServerBinaryName()
			term.PrintInfo(fmt.Sprintf("🔍 查找嵌入的RTSP Server: %s", expectedName))
			if binaries, err := embed.ListAvailableBinaries(); err == nil {
				for _, b := range binaries {
					term.PrintInfo(fmt.Sprintf("   嵌入二进制: %s", b.Name))
				}
			}

			// 优先使用嵌入的 RTSP Server（提取到 binDir）
			if embed.HasEmbeddedRtspServer() {
				term.PrintInfo("🔄 提取嵌入的RTSP Server...")
				extractedPath, err := embed.ExtractRtspServer(binDir)
				if err != nil {
					term.PrintWarning(fmt.Sprintf("提取RTSP Server失败: %v", err))
				} else {
					rtspExe = extractedPath
					rtspWorkDir = binDir // 工作目录 = 二进制所在目录（rtsp_server 读取其相对路径的同目录文件）
					term.PrintSuccess(fmt.Sprintf("✅ RTSP Server已提取到: %s", rtspExe))
				}
			} else {
				term.PrintWarning(fmt.Sprintf("⚠️ 嵌入的RTSP Server未找到 (期望: %s)", expectedName))
				term.PrintInfo("提示: go clean -cache && go build (确保embed重新扫描binaries目录)")
			}

			// 如果没有嵌入的，尝试使用外部的
			if rtspExe == "" {
				// 优先检查 binDir 中的同名二进制（嵌入时的命名规范）
				rtspExe = filepath.Join(binDir, expectedName)
				if _, err := os.Stat(rtspExe); os.IsNotExist(err) {
					// 回退到 cwd/rtsp_server（用户自行准备的二进制）
					rtspExe = filepath.Join(cwd, "rtsp_server", expectedName)
					rtspWorkDir = filepath.Join(cwd, "rtsp_server")
				} else {
					rtspWorkDir = binDir
				}
				term.PrintInfo(fmt.Sprintf("🔄 使用外部RTSP: %s", rtspExe))
			}

			// 确保 mp4path 目录存在
			os.MkdirAll(mp4Dir, 0755)
			term.PrintInfo(fmt.Sprintf("📂 MP4目录: %s", mp4Dir))

			if _, err := os.Stat(rtspExe); err == nil {
				if ln, lerr := net.Listen("tcp", ":18554"); lerr != nil {
					term.PrintWarning("⚠️ RTSP端口 18554 已被占用，跳过启动内置rtsp_server（将使用现有RTSP服务）")
				} else {
					_ = ln.Close()
					term.PrintInfo("📹 启动测试RTSP服务器...")
					rtspCmd := exec.Command(rtspExe, "1", "18554", mp4Dir)
					rtspCmd.Dir = rtspWorkDir
					var rtspLogBuf strings.Builder
					rtspCmd.Stdout = &rtspLogBuf
					rtspCmd.Stderr = &rtspLogBuf
					if err := rtspCmd.Start(); err != nil {
						term.PrintWarning(fmt.Sprintf("RTSP服务器启动失败: %v", err))
					} else {
						processes = append(processes, rtspCmd)
						term.PrintSuccess("✅ RTSP服务器已启动")
						// 启动 goroutine 监控子进程，防止僵尸进程
						go func(cmd *exec.Cmd) {
							if err := cmd.Wait(); err != nil {
								logTxt := rtspLogBuf.String()
								if strings.Contains(err.Error(), "exit status 127") ||
									strings.Contains(strings.ToLower(logTxt), "libavcodec") ||
									strings.Contains(strings.ToLower(logTxt), "error while loading shared libraries") {
									term.PrintWarning("⚠️ RTSP服务器退出: 缺少FFmpeg运行库 (libavcodec等)，视频将通过go2rtc/OWL播放")
								} else if strings.Contains(strings.ToLower(logTxt), "address already in use") || strings.Contains(strings.ToLower(logTxt), "bind") {
									term.PrintWarning("⚠️ RTSP服务器退出: 端口冲突（18554可能被占用），请检查占用后重试")
								} else if strings.Contains(strings.ToLower(logTxt), "no such file") || strings.Contains(strings.ToLower(logTxt), "cannot open") {
									term.PrintWarning("⚠️ RTSP服务器退出: 输入目录/文件异常，请检查 --mp4-dir 与视频文件")
								} else {
									term.PrintWarning(fmt.Sprintf("RTSP服务器退出: %v", err))
								}
								if strings.TrimSpace(logTxt) != "" {
									term.PrintInfo("RTSP退出日志摘录:")
									for _, line := range strings.Split(logTxt, "\n") {
										line = strings.TrimSpace(line)
										if line != "" {
											term.PrintInfo("  " + line)
										}
									}
								}
							}
						}(rtspCmd)
					}
				}
			} else {
				term.PrintWarning(fmt.Sprintf("⚠️ RTSP服务器不存在: %s", rtspExe))
				term.PrintInfo("提示: 将rtsp_server二进制放入 internal/embed/binaries/ 后执行:")
				term.PrintInfo("      go clean -cache && go build -o rivision-cli .")
			}
		}

		// 2. 启动go2rtc（可选）
		if withGo2rtc {
			var go2rtcExe string
			var go2rtcWorkDir string
			var configFile string

			// 检查外部配置文件（优先级：当前目录 > 可执行文件目录）
			// RTSP模式时跳过外部查找，由后面的自动生成逻辑接管
			var externalConfigPaths []string
			if !withRtspServer {
				externalConfigPaths = []string{
					filepath.Join(cwd, "go2rtc.yaml"),
					filepath.Join(baseDir, "go2rtc.yaml"),
				}
			}
			term.PrintInfo(fmt.Sprintf("🔍 查找配置文件 (cwd=%s, baseDir=%s)", cwd, baseDir))
			for _, p := range externalConfigPaths {
				absPath, _ := filepath.Abs(p)
				if _, err := os.Stat(absPath); err == nil {
					configFile = absPath
					term.PrintSuccess(fmt.Sprintf("📄 使用外部配置: %s", configFile))
					break
				} else {
					term.PrintInfo(fmt.Sprintf("   未找到: %s", absPath))
				}
			}

			// 优先使用嵌入的 go2rtc（提取到 binDir）
			if embed.HasEmbeddedBinary() {
				term.PrintInfo("🔄 提取嵌入的go2rtc...")
				extractedPath, err := embed.ExtractGo2rtc(binDir)
				if err != nil {
					term.PrintWarning(fmt.Sprintf("提取go2rtc失败: %v", err))
				} else {
					go2rtcExe = extractedPath
					go2rtcWorkDir = cwd // 使用当前工作目录
					// RTSP模式：始终重新生成配置（覆盖旧配置），因为旧配置可能只含RTSP源
					if withRtspServer {
						// 扫描 mp4Dir 自动生成配置到 cwd
						mp4Files := scanMp4Files(mp4Dir)
						if len(mp4Files) > 0 {
							genConfig := generateGo2rtcConfig(mp4Files, 18554, mp4Dir)
							configPath := filepath.Join(cwd, "go2rtc.yaml")
							if err := os.WriteFile(configPath, []byte(genConfig), 0644); err == nil {
								configFile = configPath
								term.PrintInfo(fmt.Sprintf("📄 自动生成配置 (%d个流): %s", len(mp4Files), configPath))
								for _, f := range mp4Files {
									name := strings.TrimSuffix(f, filepath.Ext(f))
									term.PrintInfo(fmt.Sprintf("   %s → rtsp://localhost:18554/%s", name, f))
								}
							} else {
								term.PrintWarning(fmt.Sprintf("生成配置失败: %v", err))
							}
						} else {
							term.PrintWarning("⚠️ mp4Dir 中没有 MP4 文件，go2rtc 将无流可用")
							term.PrintInfo(fmt.Sprintf("提示: 请将 MP4 文件放入 %s", mp4Dir))
						}
					} else if configFile == "" {
						// 非RTSP模式：仅在无配置时使用嵌入默认配置
						if err := embed.ExtractConfig(cwd); err != nil {
							term.PrintWarning(fmt.Sprintf("提取配置失败: %v", err))
						} else {
							configFile = filepath.Join(cwd, "go2rtc.yaml")
							term.PrintInfo("📄 使用嵌入默认配置")
						}
					}
					term.PrintSuccess(fmt.Sprintf("✅ go2rtc已提取到: %s", binDir))
				}
			}

			// 如果没有嵌入的，尝试使用外部的
			if go2rtcExe == "" {
				// 优先检查 binDir 中的同名二进制（与嵌入命名规范一致）
				go2rtcExe = filepath.Join(binDir, embed.GetGo2rtcBinaryName())
				if _, err := os.Stat(go2rtcExe); os.IsNotExist(err) {
					// 回退到 cwd/go2rtc（用户自行准备的二进制）
					go2rtcExe = filepath.Join(cwd, "go2rtc")
					go2rtcWorkDir = cwd
				} else {
					go2rtcWorkDir = binDir
				}
			}

			if _, err := os.Stat(go2rtcExe); err == nil {
				term.PrintInfo("🔄 启动go2rtc服务...")
				// 使用 -c 参数指定配置文件
				var go2rtcCmd *exec.Cmd
				if configFile != "" {
					go2rtcCmd = exec.Command(go2rtcExe, "-c", configFile)
				} else {
					go2rtcCmd = exec.Command(go2rtcExe)
				}
				go2rtcCmd.Dir = go2rtcWorkDir
				if err := go2rtcCmd.Start(); err != nil {
					term.PrintWarning(fmt.Sprintf("go2rtc启动失败: %v", err))
				} else {
					processes = append(processes, go2rtcCmd)
					term.PrintSuccess("✅ go2rtc已启动")
					// 启动 goroutine 监控子进程，防止僵尸进程
					go func(cmd *exec.Cmd) {
						if err := cmd.Wait(); err != nil {
							term.PrintWarning(fmt.Sprintf("go2rtc退出: %v", err))
						}
					}(go2rtcCmd)
				}
			} else {
				term.PrintWarning(fmt.Sprintf("⚠️ go2rtc不存在: %s", go2rtcExe))
				term.PrintInfo("提示: 请将go2rtc二进制放入 internal/embed/binaries/ 目录后重新编译")
			}
		}

		// ★ OWL (GB28181) 客户端初始化
		var owlClient *owl.Client
		if owlURL != "" {
			owlClient = owl.NewClient(owl.Config{
				Enabled:    true,
				URL:        owlURL,
				Username:   owlUsername,
				Password:   owlPassword,
				UseRSAAuth: owlRSAAuth,
			})
			term.PrintInfo(fmt.Sprintf("🔗 初始化 OWL 客户端: %s", owlURL))

			// 登录 + 能力探测（P3）
			if err := owlClient.Login(); err != nil {
				term.PrintWarning(fmt.Sprintf("⚠️ OWL 登录失败: %v", err))
			} else {
				term.PrintSuccess("✅ OWL 登录成功")
				caps := map[string]bool{"server_info": false, "channels": false, "devices": false, "devices_with_channels": false}

				if _, err := owlClient.GetServerInfo(); err == nil {
					caps["server_info"] = true
				}
				if _, _, err := owlClient.GetChannels(1, 1, ""); err == nil {
					caps["channels"] = true
				}
				if _, _, err := owlClient.GetDevices(1, 1); err == nil {
					caps["devices"] = true
				}
				if _, _, err := owlClient.GetDevicesWithChannels(1, 1); err == nil {
					caps["devices_with_channels"] = true
				}
				term.PrintInfo(fmt.Sprintf("🧪 OWL 能力探测: server_info=%v channels=%v devices=%v devices_with_channels=%v",
					caps["server_info"], caps["channels"], caps["devices"], caps["devices_with_channels"]))

				channels, err := owlClient.GetAllChannels()
				if err != nil {
					term.PrintInfo(fmt.Sprintf("ℹ️ OWL 通道列表接口不可用（%v），运行期将自动回退 go2rtc/bridge", err))
				} else {
					term.PrintInfo(fmt.Sprintf("   发现 %d 个通道", len(channels)))
					for _, ch := range channels {
						online := "离线"
						if ch.IsOnline {
							online = "在线"
						}
						name := strings.TrimSpace(ch.Name)
						if name == "" {
							name = ch.ChannelID
						}
						if name == "" {
							name = ch.ID
						}
						term.PrintInfo(fmt.Sprintf("   - [%s] %s (%s) 类型=%s", ch.ID, name, online, ch.Type))
					}
				}
			}
		}

		// 3. 初始化 YOLO 和 WebSocket 组件
		var wsHub *websocket.Hub
		var distributedDetector *yolo.DistributedDetector
		var yoloTracker *yolo.Tracker
		var useDistributedYolo bool
		var cameraManager *camera.Manager
		var go2rtcClient *camera.Go2rtcClient

		go2rtcURL := fmt.Sprintf("http://localhost:%d", go2rtcPort)

		// 创建 WebSocket Hub
		wsHub = websocket.NewHub()
		go wsHub.Run()

		// 创建 Camera Manager
		cameraManager = camera.NewManager()
		cameraManager.Start()

		// 创建 Go2rtc Client
		go2rtcClient = camera.NewGo2rtcClient(go2rtcURL)

		// ★ OWL→go2rtc 流桥接器（OWL 通道自动注册到 go2rtc）
		var owlStreamBridge *camera.OWLStreamBridge
		if owlClient != nil && owlClient.IsEnabled() {
			owlStreamBridge = camera.NewOWLStreamBridge(owlClient, go2rtcClient)
			term.PrintInfo("🔗 OWL 流桥接器已就绪（OWL → go2rtc → 浏览器）")
		}

		// 创建 YOLO 组件（如果启用）
		if enableYolo {
			term.PrintInfo("🔍 初始化 YOLO 检测器...")
			term.PrintInfo(fmt.Sprintf("   YOLO 模式: %s", yoloMode))
			// ★ 将 CLI 参数同步到 YOLOControl（供 API 读取和前端展示）
			if vlmInterval > 0 {
				webserver.GetYOLOControl().SetVLMInterval(vlmInterval)
				term.PrintInfo(fmt.Sprintf("   VLM 分析间隔: %ds", vlmInterval))
			}

			// 确定模型路径
			modelPath := yoloModel
			if !filepath.IsAbs(modelPath) {
				modelPath = filepath.Join(cwd, "models", yoloModel)
			}

			// 初始化分布式检测器
			if yoloMode == "distributed" || yoloMode == "auto" {
				term.PrintInfo("🌐 尝试初始化分布式 YOLO 检测器...")
				distributedDetector = yolo.NewDistributedDetector(yolo.DistributedConfig{
					GatewayURL: gatewayURL,
					Timeout:    5 * time.Second, // ★ K3 warmup 1-3s, 正常推理 20-100ms; 需 > Gateway httpx 超时(5s); 旧值 10s 太长
					Retry:      1,
				})
				if distributedDetector.IsEnabled() {
					useDistributedYolo = true
					term.PrintSuccess(fmt.Sprintf("✅ 分布式 YOLO 已启用: %s", gatewayURL))
					// ★ 启动连通性检查：验证 Gateway + YOLO 节点是否可达
					if online, latMs, err := distributedDetector.CheckGatewayStatus(); err != nil {
						term.PrintWarning(fmt.Sprintf("⚠️ Gateway 连接失败: %v", err))
					} else if online {
						term.PrintSuccess(fmt.Sprintf("✅ Gateway 在线 (延迟 %dms)", latMs))
						if n, err := distributedDetector.GetHealthyNodeCount(); err == nil {
							term.PrintInfo(fmt.Sprintf("   YOLO 可用节点: %d", n))
							if n == 0 {
								term.PrintWarning("⚠️ 没有健康的 YOLO 节点！请检查节点上 rivision-yolo 服务是否运行")
							}
						}
					}
				} else {
					term.PrintWarning("分布式 YOLO 未配置 Gateway URL")
				}
			}

			// 创建跟踪器
			yoloTracker = yolo.NewTracker(0.5, 0.3, 30)
		}

		// 设置帧回调 → YOLO 检测 → WebSocket 广播
		yoloReady := useDistributedYolo
		
		if yoloReady {
			// ★★★ YOLO 检测管理器：Fan-out 流水线架构 ★★★
			//
			// 架构设计：
			//   [Capture goroutine/cam] → jobCh → [Worker pool (N个)] → resultCh → [Processor]
			//
			// 关键设计原则：
			//   1. 每帧只抓取一次、只检测一次（无重复工作）
			//   2. 不同帧分发到不同节点并行处理（真正的分布式并行）
			//   3. 单一结果处理器保证 Tracker 输入无竞争（无需锁）
			//   4. Worker pool 大小 = 节点数，自动扩缩容
			//
			yoloDone := make(chan struct{})
			go func() {
				defer close(yoloDone)
				term.PrintInfo("⏳ 等待视频流初始化...")
				time.Sleep(3 * time.Second)

				// ─── 数据类型 ───
				type yoloJob struct {
					streamName string
					frame      *camera.Frame
				}
				type yoloResultMsg struct {
					streamName string
					frame      *camera.Frame
					result     *yolo.DetectionResult
					frameTime  float64
					detectErr  error
				}

				// ─── 通道 ───
				jobCh := make(chan yoloJob, 16)        // ★ 缓冲=16: 减少帧排队延迟，提升绘制同步性（K3 5-30fps 场景）
				resultCh := make(chan yoloResultMsg, 16) // worker → 结果处理器（与 jobCh 匹配）
				cancelCh := make(chan struct{})        // 全局取消信号

				// ─── 全局计数器（atomic，多 goroutine 安全） ───
				var totalFrames int64
				var totalErrors int64
				var activeWorkers int32
				var desiredWorkers int32 // ★ 期望 worker 数（用于双向缩扩容）
				lastKnownNodes := 1
				const workersPerNode = 4 // ★ 每节点并发 worker 数（隐藏网络延迟）
				// ★ 吞吐量测量（用于自适应帧率）
				var lastMeasuredFrames int64
				lastMeasureTime := time.Now()
				atomic.StoreInt32(&desiredWorkers, int32(lastKnownNodes*workersPerNode))

				// ─── 层1: Worker Pool（检测层，大小 = 节点数 × 并发度） ───
				// 每个 worker 从 jobCh 取帧 → 发到 Gateway → 结果送 resultCh
				// 多 worker 并发请求同一节点，隐藏网络 RTT 和编码开销
				var startWorker func(id int)
				startWorker = func(id int) {
					atomic.AddInt32(&activeWorkers, 1)
					go func() {
						defer atomic.AddInt32(&activeWorkers, -1)
						defer func() {
							if r := recover(); r != nil {
								fmt.Printf("[YOLO] ⚠️ Worker-%d panic: %v，2秒后重启\n", id, r)
								time.Sleep(2 * time.Second)
								select {
								case <-cancelCh:
									return
								default:
									// ★ 仅在需要更多 worker 时才重启
									if atomic.LoadInt32(&activeWorkers) < atomic.LoadInt32(&desiredWorkers) {
										startWorker(id)
									}
								}
							}
						}()
						var workerFrameCount int64 // ★ per-worker 本地帧计数（无竞争）
						for {
							// ★ 缩容检查：处理完任务后，如果 worker 数超出期望，主动退出
							if atomic.LoadInt32(&activeWorkers) > atomic.LoadInt32(&desiredWorkers) {
								fmt.Printf("[YOLO] ⬇️ Worker-%d 退出 (缩容: active=%d > desired=%d)\n",
									id, atomic.LoadInt32(&activeWorkers), atomic.LoadInt32(&desiredWorkers))
								return
							}
							select {
							case <-cancelCh:
								return
							case job, ok := <-jobCh:
								if !ok {
									return
								}
								var result *yolo.DetectionResult
								var detectErr error

								detectStart := time.Now()
								if useDistributedYolo {
									result, detectErr = distributedDetector.Detect(job.frame.Data, job.frame.Width, job.frame.Height)
								}
								detectMs := time.Since(detectStart).Milliseconds()
								workerFrameCount++

								// ★ 诊断日志（每 worker 每 10 帧输出一次）
								if workerFrameCount%10 == 1 {
									nDets := 0
									if result != nil {
										nDets = len(result.Detections)
									}
									fmt.Printf("[YOLO] ⏱️ W%d #%d %s: detect=%dms size=%dKB %dx%d dets=%d\n",
										id, workerFrameCount, job.streamName, detectMs, len(job.frame.Data)/1024,
										job.frame.Width, job.frame.Height, nDets)
								}

								if detectErr != nil {
									atomic.AddInt64(&totalErrors, 1)
									result = &yolo.DetectionResult{
										Detections: []yolo.Detection{},
										ImageWidth: job.frame.Width, ImageHeight: job.frame.Height,
									}
								}
								if result == nil {
									result = &yolo.DetectionResult{
										Detections: []yolo.Detection{},
										ImageWidth: job.frame.Width, ImageHeight: job.frame.Height,
									}
								}
								result.CameraID = job.streamName
								result.FrameIndex = job.frame.Index

								select {
								case resultCh <- yoloResultMsg{
									streamName: job.streamName,
									frame:      job.frame,
									result:     result,
									frameTime:  float64(time.Now().UnixNano()) / 1e9,
									detectErr:  detectErr,
								}:
								case <-cancelCh:
									return
								}
							}
						}
					}()
				}

				// 启动初始 worker（节点数 × 并发度）
				initialWorkers := lastKnownNodes * workersPerNode
				for i := 0; i < initialWorkers; i++ {
					startWorker(i)
				}
				fmt.Printf("[YOLO] 🔧 Worker pool 启动: %d workers (%d节点 × %d并发)\n", initialWorkers, lastKnownNodes, workersPerNode)

				// ─── 层2: 结果处理器（单 goroutine，无 Tracker 竞争，panic 自动重启） ───
				var startProcessor func()
				startProcessor = func() {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								fmt.Printf("[YOLO] ⚠️ 结果处理器 panic: %v，2秒后重启\n", r)
								time.Sleep(2 * time.Second)
								select {
								case <-cancelCh:
									return
								default:
									startProcessor()
								}
							}
						}()
						for {
							select {
							case <-cancelCh:
								return
							case res, ok := <-resultCh:
								if !ok {
									return
								}

								// ★ 错误帧快速路径：跳过昂贵的 JPEG decode/draw/encode
								if res.detectErr != nil {
									errCount := atomic.LoadInt64(&totalErrors)
									if errCount <= 5 || errCount%100 == 0 {
										fmt.Printf("[YOLO] ❌ %s: 检测失败 (#%d): %v\n", res.streamName, errCount, res.detectErr)
									}
									// 错误帧: JSON-only（与正常路径一致，无 BinaryData）
									msg := websocket.NewCameraMessage(
										websocket.TypeYOLODetection,
										res.streamName,
										websocket.YOLODetectionData{
											Detections:      []websocket.Detection{},
											InferenceTimeMs: 0,
											FrameTime:       res.frameTime,
											ImageWidth:      res.frame.Width,
											ImageHeight:     res.frame.Height,
										},
									)
									wsHub.SendToChannel("yolo:"+res.streamName, msg)
									atomic.AddInt64(&totalFrames, 1)
									continue
								}

								// 应用跟踪（单 goroutine，无锁竞争）
								if yoloTracker != nil {
									res.result.Detections = yoloTracker.UpdateWithTime(res.result.Detections, res.frameTime)
								}

								// 归一化坐标
								imgW := float64(res.frame.Width)
								imgH := float64(res.frame.Height)
								detections := make([]websocket.Detection, len(res.result.Detections))
								for i, d := range res.result.Detections {
									detections[i] = websocket.Detection{
										ClassID:    d.ClassID,
										ClassName:  d.ClassName,
										Confidence: d.Confidence,
										BBox:       [4]float64{float64(d.BBox.X1) / imgW, float64(d.BBox.Y1) / imgH, float64(d.BBox.X2) / imgW, float64(d.BBox.Y2) / imgH},
										TrackID:    d.TrackID,
										VelocityX:  float64(d.VelocityX) / imgW,
										VelocityY:  float64(d.VelocityY) / imgH,
									}
								}

								// ★ 性能优化:
								// 1. 跳过后端 DrawDetectionsOnFrame（K3 上 ~200-800ms/帧）
								// 2. 不发送二进制 JPEG（Smooth模式不需要，省 ~3MB/s @4路15fps）
								// 前端 Canvas 统一绘制检测框（Smooth/Sync 两种模式）

								// WebSocket 广播: 仅 JSON 检测数据（~0.5KB vs 50KB+）
								msg := websocket.NewCameraMessage(
									websocket.TypeYOLODetection,
									res.streamName,
									websocket.YOLODetectionData{
										Detections:      detections,
										InferenceTimeMs: res.result.InferenceTimeMs,
										FrameIndex:      res.result.FrameIndex,
										FrameTime:       res.frameTime,
										ImageWidth:      res.frame.Width,
										ImageHeight:     res.frame.Height,
									},
								)
								// ★ 无 BinaryData → WritePump 只发 JSON (~0.5ms vs ~2.5ms)
								wsHub.SendToChannel("yolo:"+res.streamName, msg)

								seq := atomic.AddInt64(&totalFrames, 1)
								// ★ 诊断：结果处理器耗时（每 20 帧输出）
								if seq%20 == 0 {
									fmt.Printf("[YOLO] 📦 结果处理 #%d: dets=%d %s\n",
										seq, len(detections), res.streamName)
								}
							}
						}
					}()
				}
				startProcessor()

				// ─── 层3: Per-camera 抓帧 goroutine ───
				// 每个摄像头一个 goroutine，只负责抓帧并送入 jobCh
				// 抓帧与检测解耦 → 检测可并行分发到不同节点
				activeCameras := make(map[string]chan struct{}) // streamName → stopCh

				var captureCamera func(streamName string, stopCh chan struct{})
				captureCamera = func(streamName string, stopCh chan struct{}) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Printf("[YOLO] ⚠️ %s: 抓帧 goroutine panic: %v，2秒后重启\n", streamName, r)
							time.Sleep(2 * time.Second)
							// 检查是否已取消（系统关闭时不重启）
							select {
							case <-cancelCh:
								return
							case <-stopCh:
								return
							default:
								go captureCamera(streamName, stopCh)
							}
						}
					}()

					fmt.Printf("[YOLO] 🟢 %s: 抓帧循环启动\n", streamName)
					var consecutiveErrors int
					var captureCount int64

					for {
						select {
						case <-stopCh:
							fmt.Printf("[YOLO] 🔴 %s: 抓帧循环停止\n", streamName)
							return
						case <-cancelCh:
							return
						default:
						}

						iterStart := time.Now()

						// 检查是否仍然启用
						if !webserver.GetYOLOControl().IsEnabled(streamName) {
							return
						}

						// ★ 后端抓帧禁用时休眠（前端 DirectDetector 替代）
						if !webserver.GetYOLOControl().IsBackendCaptureEnabled() {
							time.Sleep(5 * time.Second)
							continue
						}

						// ★ 从 go2rtc 获取降分辨率帧（YOLO 模型输入通常 640×640）
						// 1280×720 → 640×360: 数据量减 ~75%, K3 推理加速 4-10×
						captureStart := time.Now()
						frame, err := go2rtcClient.CaptureFrameResized(streamName, 640)
						captureMs := time.Since(captureStart).Milliseconds()
						if err != nil {
							consecutiveErrors++
							if consecutiveErrors <= 3 || consecutiveErrors%50 == 0 {
								fmt.Printf("[YOLO] %s: 获取帧失败 (%d连续, %dms): %v\n", streamName, consecutiveErrors, captureMs, err)
							}
							time.Sleep(50 * time.Millisecond) // ★ 快速重试（50ms 而非 200ms）
							continue
						}
						consecutiveErrors = 0
						// ★ 抓帧耗时诊断（每 20 帧输出一次）
						captureCount++
						if captureCount%20 == 1 {
							fmt.Printf("[YOLO] 📷 %s: capture=%dms size=%dKB %dx%d (#%d)\n",
								streamName, captureMs, len(frame.Data)/1024,
								frame.Width, frame.Height, captureCount)
						}

						// 送入 worker pool（非阻塞：pool 满则跳过该帧，保持最新性）
						select {
						case jobCh <- yoloJob{streamName: streamName, frame: frame}:
							// 成功送入
						default:
							// worker pool 满，丢弃此帧（保持实时性，不堆积旧帧）
						}

						// ★ 帧率控制：使用配置的帧率上限
						targetFPS := webserver.GetYOLOControl().GetSyncFrameRate()
						if targetFPS <= 0 {
							targetFPS = 15
						}
						minInterval := time.Duration(1000/targetFPS) * time.Millisecond
						elapsed := time.Since(iterStart)
						if elapsed < minInterval {
							time.Sleep(minInterval - elapsed)
						}
					}
				}

				// ─── 管理循环：摄像头启停 + Worker 自动扩缩容 ───
				heartbeatTicker := time.NewTicker(30 * time.Second)
				pollTicker := time.NewTicker(1 * time.Second)
				nodeCheckTicker := time.NewTicker(10 * time.Second)
				defer heartbeatTicker.Stop()
				defer pollTicker.Stop()
				defer nodeCheckTicker.Stop()

				term.PrintSuccess("✅ YOLO 检测管理器已启动 (fan-out pipeline)")

				for {
					select {
					case <-sigChan:
						// 关闭顺序：1.取消信号 → 2.停止抓帧 → 3.workers自动退出
						close(cancelCh)
						for name, stopCh := range activeCameras {
							close(stopCh)
							fmt.Printf("[YOLO] 停止 %s\n", name)
						}
						return

					case <-heartbeatTicker.C:
						workers := atomic.LoadInt32(&activeWorkers)
						frames := atomic.LoadInt64(&totalFrames)
						errors := atomic.LoadInt64(&totalErrors)
						fmt.Printf("[YOLO] 💓 心跳: %d 摄像头, %d workers, %d 节点, %d 总帧, %d 总错误\n",
							len(activeCameras), workers, lastKnownNodes, frames, errors)
						// ★ 同步 Pipeline 统计到 YOLOControl（供前端 /api/yolo/pipeline-stats 读取）
						yoloCtrlStats := webserver.GetYOLOControl()
						atomic.StoreInt32(&yoloCtrlStats.ActiveWorkers, workers)
						atomic.StoreInt32(&yoloCtrlStats.DesiredWorkers, atomic.LoadInt32(&desiredWorkers))
						atomic.StoreInt32(&yoloCtrlStats.HealthyNodes, int32(lastKnownNodes))
						atomic.StoreInt64(&yoloCtrlStats.TotalFrames, frames)
						atomic.StoreInt64(&yoloCtrlStats.TotalErrors, errors)

					case <-nodeCheckTicker.C:
						// 查询节点数 → 自动扩缩容 worker pool
						if useDistributedYolo && distributedDetector != nil {
							if n, err := distributedDetector.GetHealthyNodeCount(); err == nil && n > 0 {
								if n != lastKnownNodes {
									fmt.Printf("[YOLO] 📊 节点数变化: %d → %d\n", lastKnownNodes, n)
									lastKnownNodes = n
								}
							}
						}
						// ★ 更新期望 worker 数：max(摄像头数, 节点数×并发度)
						numCams := len(activeCameras)
						targetWorkers := lastKnownNodes * workersPerNode
						if numCams > targetWorkers {
							targetWorkers = numCams
						}
						if targetWorkers < 1 {
							targetWorkers = 1
						}
						atomic.StoreInt32(&desiredWorkers, int32(targetWorkers))

						// 扩容：当前 worker 不够时补充
						current := int(atomic.LoadInt32(&activeWorkers))
						if current < targetWorkers {
							for i := current; i < targetWorkers; i++ {
								startWorker(i)
							}
							fmt.Printf("[YOLO] ⬆️ Worker pool 扩容: %d → %d (N=%d, M=%d)\n", current, targetWorkers, lastKnownNodes, numCams)
						}
						// 缩容：由 worker 自身在循环顶部检查 activeWorkers > desiredWorkers 后退出

						// ★ 自适应帧率: 基于实测吞吐量动态调整（K3 5-30fps 适配）
						numCameras := len(activeCameras)
						if numCameras > 0 {
							now := time.Now()
							deltaSec := now.Sub(lastMeasureTime).Seconds()
							if deltaSec >= 5.0 { // 每 ≥5 秒测量一次吞吐
								curFrames := atomic.LoadInt64(&totalFrames)
								throughput := float64(curFrames-lastMeasuredFrames) / deltaSec
								lastMeasuredFrames = curFrames
								lastMeasureTime = now

								// 计算每摄像头最优帧率
								var optimalFPS int
								if throughput > 0 {
									// 实测吞吐 / 摄像头数 × 0.9（留 10% 余量避免过载）
									optimalFPS = int(throughput * 0.9 / float64(numCameras))
								} else {
									optimalFPS = 5 // 初始保守值
								}
								// 限制范围: 2-60fps
								if optimalFPS < 2 {
									optimalFPS = 2
								}
								if optimalFPS > 60 {
									optimalFPS = 60
								}

								currentFPS := webserver.GetYOLOControl().GetSyncFrameRate()
								// 仅在差异 ≥2fps 时调整（避免频繁抖动）
								diff := optimalFPS - currentFPS
								if diff < 0 { diff = -diff }
								if diff >= 2 {
									webserver.GetYOLOControl().SetSyncFrameRate(optimalFPS)
									fmt.Printf("[YOLO] 📐 自适应帧率: %d → %dfps (吞吐=%.1ffps N=%d M=%d)\n",
										currentFPS, optimalFPS, throughput, lastKnownNodes, numCameras)
								}
							}
						}

					case <-pollTicker.C:
						// ★ 每秒同步 Pipeline 统计（供前端实时展示）
						yoloCtrl := webserver.GetYOLOControl()
						atomic.StoreInt32(&yoloCtrl.ActiveWorkers, atomic.LoadInt32(&activeWorkers))
						atomic.StoreInt32(&yoloCtrl.DesiredWorkers, atomic.LoadInt32(&desiredWorkers))
						atomic.StoreInt32(&yoloCtrl.HealthyNodes, int32(lastKnownNodes))
						atomic.StoreInt64(&yoloCtrl.TotalFrames, atomic.LoadInt64(&totalFrames))
						atomic.StoreInt64(&yoloCtrl.TotalErrors, atomic.LoadInt64(&totalErrors))

						enabledCameras := yoloCtrl.GetEnabledCameras()
						enabledSet := make(map[string]bool, len(enabledCameras))
						for _, name := range enabledCameras {
							enabledSet[name] = true
						}

						// 停止已禁用的摄像头
						for name, stopCh := range activeCameras {
							if !enabledSet[name] {
								close(stopCh)
								delete(activeCameras, name)
								fmt.Printf("[YOLO] 🔴 %s: 已禁用，停止抓帧\n", name)
							}
						}

						// 启动新启用的摄像头
						for _, name := range enabledCameras {
							if _, exists := activeCameras[name]; !exists {
								stopCh := make(chan struct{})
								activeCameras[name] = stopCh
								go captureCamera(name, stopCh)
							}
						}
					}
				}
			}()
			term.PrintSuccess("✅ YOLO 检测管理器已启动")
		}

// 4. 初始化语义索引（SQLite）
// 使用 rivision-embed 服务（Chinese-CLIP）进行 embedding
embedURL := strings.TrimSpace(os.Getenv("RIVISION_EMBED_URL"))
enableRemote := strings.TrimSpace(os.Getenv("RIVISION_EMBED_REMOTE_URL")) != ""
if runtime.GOARCH == "riscv64" {
enableRemote = false
}

// 自动检测本地 rivision-embed 服务（默认端口 18081）
if embedURL == "" {
	defaultEmbedURL := "http://127.0.0.1:18081"
	resp, err := http.Get(defaultEmbedURL + "/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			embedURL = defaultEmbedURL
			term.PrintInfo(fmt.Sprintf("🔍 自动检测到 rivision-embed 服务: %s", embedURL))
		}
	}
}

if embedURL != "" {
term.PrintSuccess(fmt.Sprintf("✅ 使用 rivision-embed 服务: %s", embedURL))
}
		semantic.ConfigureProviderRuntime(semantic.RuntimeOptions{
			RemoteURL:    os.Getenv("RIVISION_EMBED_REMOTE_URL"),
			RemoteAPIKey: os.Getenv("RIVISION_EMBED_REMOTE_KEY"),
			EnableRemote: enableRemote,
			EmbedURL:     embedURL,
			EmbedAPIKey:  os.Getenv("RIVISION_EMBED_API_KEY"),
		})
		st := semantic.ProviderStatusSnapshot()
		term.PrintInfo(fmt.Sprintf("🧠 Embedding Provider: active=%s chain=%v degraded=%v", st.ActiveProvider, st.ProviderChain, st.Degraded))

		semanticStore, err := semantic.NewStore(filepath.Join(cwd, "data"))
		if err != nil {
			term.PrintWarning(fmt.Sprintf("⚠️ 初始化语义索引失败: %v", err))
		} else {
			term.PrintSuccess("✅ 语义索引已启用 (SQLite)")
			defer semanticStore.Close()
		}

		// 5. 启动Web服务器
		term.PrintInfo(fmt.Sprintf("🌐 启动Web服务器 (端口: %d)...", webuiPort))

		// 显示访问信息
		term.PrintSeparator()
		term.PrintSuccess("🚀 服务已启动!")
		term.PrintInfo(fmt.Sprintf("Web界面:     http://localhost:%d", webuiPort))
		term.PrintInfo(fmt.Sprintf("API文档:     http://localhost:%d/docs", webuiPort))
		term.PrintInfo(fmt.Sprintf("Gateway:     %s", gatewayURL))
		term.PrintInfo(fmt.Sprintf("go2rtc:      %s", go2rtcURL))
		if withRtspServer {
			term.PrintInfo("测试RTSP:    rtsp://localhost:18554/{filename}")
			term.PrintInfo(fmt.Sprintf("MP4目录:     %s", mp4Dir))
		}
		term.PrintSeparator()
		term.PrintInfo("\n按 Ctrl+C 停止所有服务\n")

		// 打开浏览器
		if !noBrowser {
			openBrowser(fmt.Sprintf("http://localhost:%d", webuiPort))
		}

		// 启动Web服务器（阻塞）
		server := webserver.New(webserver.Config{
			Port:            webuiPort,
			GatewayURL:      gatewayURL,
			Go2rtcURL:       go2rtcURL,
			CameraManager:   cameraManager,
			YOLOTracker:     yoloTracker,
			WSHub:           wsHub,
			OWLClient:       owlClient,
			OWLStreamBridge: owlStreamBridge,
			SemanticStore:   semanticStore,
		})

		// 注意: SetupRoutes 中的 API 路由与 server.go 中的 /api/*path 代理冲突
		// YOLO WebSocket 已在 server.go 中单独注册，无需再调用 SetupRoutes
		_ = cameraManager // 避免未使用警告

		// 在goroutine中运行服务器
		errChan := make(chan error, 1)
		go func() {
			errChan <- server.Run()
		}()

		// 等待信号或错误
		select {
		case <-sigChan:
			term.PrintWarning("\n正在停止服务...")
		case err := <-errChan:
			if err != nil {
				term.PrintError(fmt.Sprintf("Web服务器错误: %v", err))
			}
		}

		// 停止所有子进程
		for _, proc := range processes {
			if proc.Process != nil {
				proc.Process.Kill()
			}
		}
		term.PrintSuccess("✅ 所有服务已停止")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(webuiCmd)
	webuiCmd.Flags().IntVar(&webuiPort, "port", 8280, "Web服务器端口")
	webuiCmd.Flags().IntVar(&go2rtcPort, "go2rtc-port", 1984, "go2rtc端口")
	webuiCmd.Flags().BoolVar(&withRtspServer, "with-rtsp-server", false, "同时启动测试RTSP服务器")
	webuiCmd.Flags().BoolVar(&withGo2rtc, "with-go2rtc", false, "同时启动go2rtc服务")
	webuiCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "不自动打开浏览器")
	webuiCmd.Flags().StringVar(&mp4Dir, "mp4-dir", "./mp4path", "MP4视频文件目录")

	// 新增参数
	webuiCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	webuiCmd.Flags().BoolVar(&enableYolo, "enable-yolo", false, "启用 YOLO 检测")
	webuiCmd.Flags().StringVar(&yoloModel, "yolo-model", "yolov8n.onnx", "YOLO 模型路径")
	webuiCmd.Flags().IntVar(&vlmInterval, "vlm-interval", 12, "VLM 分析间隔(秒), K3推理4-10s故默认12s")
	webuiCmd.Flags().StringVar(&vlmTriggerMode, "vlm-trigger", "interval", "VLM 触发模式 (interval/yolo/manual)")
	webuiCmd.Flags().StringVar(&yoloMode, "yolo-mode", "auto", "YOLO 模式 (local/distributed/auto)")

	// OWL (GB28181) 参数 - 默认连接本地 OWL
	webuiCmd.Flags().StringVar(&owlURL, "owl-url", "http://127.0.0.1:15123", "OWL 服务地址 (空字符串禁用)")
	webuiCmd.Flags().StringVar(&owlUsername, "owl-user", "admin", "OWL 用户名")
	webuiCmd.Flags().StringVar(&owlPassword, "owl-pass", "admin", "OWL 密码")
	webuiCmd.Flags().BoolVar(&owlRSAAuth, "owl-rsa", false, "OWL 使用 RSA 加密登录 (生产环境)")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch {
	case isCommandAvailable("xdg-open"):
		cmd = exec.Command("xdg-open", url)
	case isCommandAvailable("open"):
		cmd = exec.Command("open", url)
	case isCommandAvailable("start"):
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	cmd.Start()
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// scanMp4Files 扫描目录中的 MP4 文件，返回文件名列表
func scanMp4Files(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".mp4" {
			files = append(files, name)
		}
	}
	return files
}

// generateGo2rtcConfig 根据 MP4 文件列表生成 go2rtc 配置
// 使用 RTSP URL 指向内嵌的 RTSP 服务器（go2rtc 内置 RTSP 客户端，无需外部 ffmpeg）。
// RTSP 服务器若因 FFmpeg 版本不匹配退出时，go2rtc 仍可正常播放。
func generateGo2rtcConfig(mp4Files []string, rtspPort int, mp4Dir string) string {
	var sb strings.Builder
	sb.WriteString("# go2rtc 自动生成配置（基于 mp4Dir 扫描结果）\n\n")
	sb.WriteString("api:\n  listen: \":1984\"\n  origin: \"*\"\n\n")
	sb.WriteString("rtsp:\n  listen: \":8554\"\n  default_query: \"video&audio\"\n\n")
	sb.WriteString("webrtc:\n  candidates:\n    - stun:stun.l.google.com:19302\n\n")
	sb.WriteString("streams:\n")
	for _, f := range mp4Files {
		name := strings.TrimSuffix(f, filepath.Ext(f))
		// go2rtc 内置 RTSP 客户端直接拉流，无需外部 ffmpeg
		sb.WriteString(fmt.Sprintf("  %s: rtsp://admin:123456@localhost:%d/%s\n", name, rtspPort, f))
	}
	sb.WriteString("\nlog:\n  level: info\n")
	return sb.String()
}
