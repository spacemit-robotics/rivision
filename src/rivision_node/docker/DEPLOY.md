# RiVision Docker 部署指南

## 系统架构

RiVision 由 3 个 Docker 镜像组成，分布在 Server K3 和 Node K3 上：

```
┌─── Server K3 (1台) ─────────────────────────────────┐
│                                                       │
│  rivision-server (Nginx + FastAPI Backend)  :80       │
│  ├── 前端 (Vue3)                                      │
│  ├── 后端 (FastAPI + SQLite)                          │
│  └── 嵌入模型服务                                     │
│                                                       │
│  inference-gateway (FastAPI)               :8081      │
│  ├── 节点注册/注销                                     │
│  ├── 负载均衡 (least_conn)                             │
│  ├── 健康检查                                          │
│  ├── 异步任务队列                                      │
│  └── 节点身份认证                                      │
│                                                       │
│  ⚠️ Gateway 端口 8081 必须对 Node K3 可达              │
└───────────────────────────────────────────────────────┘
         ↕ HTTP (GATEWAY_URL)
┌─── Node K3 (N台) ────────────────────────────────────┐
│                                                       │
│  rivision-node (supervisord)                          │
│  ├── llama-server (llama.cpp 多模态推理)    :9080     │
│  └── node-agent   (FastAPI 节点代理)        :9090     │
│      ├── 启动时向 Gateway 注册 (NODE_ID + TOKEN)      │
│      ├── 定期心跳上报                                  │
│      └── 管理 llama-server 生命周期                    │
│                                                       │
│  每台 K3 通过 .env 配置不同的:                         │
│  NODE_ID / REGISTRATION_TOKEN / GATEWAY_URL           │
└───────────────────────────────────────────────────────┘
```

### Docker 镜像清单

| 镜像 | 位置 | Dockerfile | 端口 | 说明 |
|------|------|-----------|------|------|
| `rivision-server` | Server K3 | `Dockerfile.combined.k3` | 80 | Nginx + Backend + Embedding |
| `rivision-gateway` | Server K3 | `Dockerfile.gateway.k3` | 8081 | 推理网关 (节点注册/负载均衡) |
| `rivision-node` | Node K3 | `Dockerfile.node.k3` | 9080, 9090 | llama-server + node-agent |

## 部署概述

```
x86 开发机                构建 K3                    生产 K3 (N台)
──────────               ─────────                  ──────────────
源码包 (32K) ──scp──→    编译+构建                   
                         ├ llama.cpp 编译             
                         ├ docker build               
                         └ 打包部署包 ──scp──→        docker load + up
                           ~7GB (含模型)
```

---

## Phase 1: x86 开发机打包

```bash
# 在 rivision_node/ 目录下
tar czf rivision_node_src.tgz \
    docker/ .dockerignore node-agent/ scripts/ config/ VERSION

# 复制到构建 K3
scp rivision_node_src.tgz user@K3_BUILD_IP:/home/user/
```

---

## Phase 2: 构建 K3 (一次性)

### 2.1 解压源码包

```bash
# K3 上已有 rivision_node 目录 (之前 install.sh 部署的)
cd /home/user/rivision_node   # 或实际路径
tar xzf ../rivision_node_src.tgz
```

### 2.2 编译 llama.cpp (如未编译)

```bash
# 检查是否已编译
ls llama.cpp/build-k3/bin/llama-server

# 如果不存在，编译 (~5-10分钟)
bash scripts/build.sh

# 验证编译产物
ls -lh llama.cpp/build-k3/bin/
# 应包含: llama-server, llama-mtmd-cli, llama-quantize, lib*.so*
```

### 2.3 清理编译中间产物 (可选, 节省 129MB)

```bash
# 只保留 bin/ 目录，删除 .o/.cmake 等
cd llama.cpp/build-k3
find . -maxdepth 1 -type d ! -name bin ! -name . -exec rm -rf {} +
rm -f *.cmake Makefile CMakeCache.txt cmake_install.cmake
cd ../..
# 清理后 build-k3/ 从 144MB → 15MB
```

### 2.4 配置 .env

```bash
cd docker
cp .env.example .env
vim .env
# 必改:
#   NODE_ID=k3-build-01          (唯一节点名)
#   GATEWAY_URL=http://x.x.x.x:8081  (Server IP)
#   REGISTRATION_TOKEN=xxx       (注册令牌)
```

### 2.5 Docker 构建镜像

```bash
# 在 rivision_node/ 根目录执行 (不是 docker/ 目录)
docker compose -f docker/docker-compose.node.k3.yml build

# 验证镜像
docker images rivision-node
```

### 2.6 测试运行

```bash
docker compose -f docker/docker-compose.node.k3.yml up -d
docker logs -f rivision-node

# 健康检查
curl http://localhost:9090/api/v1/health
curl http://localhost:9080/health

# 停止
docker compose -f docker/docker-compose.node.k3.yml down
```

### 2.7 创建部署包 (用于其他 K3)

```bash
# 1. 导出 Docker 镜像
docker save rivision-node:latest | gzip > rivision-node-image.tar.gz
echo "镜像大小: $(ls -lh rivision-node-image.tar.gz | awk '{print $5}')"

# 2. 创建部署包目录
mkdir -p /tmp/rivision_node_deploy/{docker,config,models}

# 复制必要文件
cp docker/docker-compose.node.k3.yml /tmp/rivision_node_deploy/docker/
cp docker/.env.example              /tmp/rivision_node_deploy/docker/
cp config/models.conf               /tmp/rivision_node_deploy/config/
cp rivision-node-image.tar.gz       /tmp/rivision_node_deploy/

# 复制模型文件 (根据需要选择)
cp models/fastvlm-0.5b-q8_0.gguf      /tmp/rivision_node_deploy/models/
cp models/fastvlm-0.5b-mmproj-f16.gguf /tmp/rivision_node_deploy/models/
# cp models/其他模型...

# 3. 打包
cd /tmp
tar czf rivision_node_deploy.tgz rivision_node_deploy/
echo "部署包大小: $(ls -lh rivision_node_deploy.tgz | awk '{print $5}')"
# 预估: ~200MB (镜像) + 模型文件大小
```

---

## Phase 3: 生产 K3 部署

```bash
# 1. 复制部署包到生产 K3
scp rivision_node_deploy.tgz user@K3_PROD_IP:/opt/

# 2. 解压
ssh user@K3_PROD_IP
cd /opt
tar xzf rivision_node_deploy.tgz
mv rivision_node_deploy rivision_node

# 3. 加载 Docker 镜像
docker load < /opt/rivision_node/rivision-node-image.tar.gz
# 可删除 tar.gz 释放空间
rm /opt/rivision_node/rivision-node-image.tar.gz

# 4. 配置 (每台 K3 不同!)
cd /opt/rivision_node/docker
cp .env.example .env
vim .env
#   NODE_ID=k3-prod-01
#   GATEWAY_URL=http://server-ip:8081
#   REGISTRATION_TOKEN=your-token

# 5. 创建日志目录
mkdir -p /opt/rivision_node/logs

# 6. 启动
docker compose -f docker-compose.node.k3.yml up -d

# 7. 验证
docker logs rivision-node
curl http://localhost:9090/api/v1/health
```

---

## Phase 4: 离线部署 (Server + Gateway + Node 完整包)

### 系统组件说明

| 组件 | 镜像名 | 部署位置 | compose 文件 | 端口 |
|------|--------|---------|-------------|------|
| **rivision-server** | `rivision-server:latest` | Server K3 | `docker-compose.combined.k3.yml` | :80 |
| **inference-gateway** | `rivision-gateway:latest` | Server K3 | `docker-compose.combined.k3.yml` | :8081 |
| **rivision-node** | `rivision-node:latest` | Node K3 (每台) | `docker-compose.node.k3.yml` | :9080, :9090 |

> ⚠️ `rivision-server` 和 `inference-gateway` 在同一个 `docker-compose.combined.k3.yml` 中定义，
> 共用 `rivision-net` Docker 网络。Gateway 端口 8081 必须暴露给 Node K3 访问。

### 导出所有镜像

```bash
# ── Server K3 上导出 (2个镜像) ──
docker save rivision-server:latest rivision-gateway:latest | gzip > rivision-server-images.tar.gz

# ── Node K3 上导出 (1个镜像) ──
docker save rivision-node:latest | gzip > rivision-node-image.tar.gz

# 或在同一台 K3 上合并导出所有 3 个镜像
docker save rivision-server:latest rivision-gateway:latest rivision-node:latest \
    | gzip > rivision-all-images.tar.gz
```

### 创建离线部署包

```bash
mkdir -p rivision_offline/{node/{docker,config,models},server/docker}

# ── Node 部署文件 ──
cp rivision-node-image.tar.gz              rivision_offline/node/
cp node/docker/docker-compose.node.k3.yml  rivision_offline/node/docker/
cp node/docker/.env.example                rivision_offline/node/docker/
cp node/config/models.conf                 rivision_offline/node/config/
cp node/models/*.gguf                      rivision_offline/node/models/

# ── Server 部署文件 (含 inference-gateway) ──
cp rivision-server-images.tar.gz           rivision_offline/server/
cp server/docker/docker-compose.combined.k3.yml  rivision_offline/server/docker/
cp server/docker/.env.example              rivision_offline/server/docker/

# ── 打包 ──
tar czf rivision_offline_deploy.tgz rivision_offline/
```

### 离线部署: Server K3

```bash
# 1. 解压
tar xzf rivision_offline_deploy.tgz
cd rivision_offline/server

# 2. 加载镜像 (rivision-server + rivision-gateway 共 2 个)
docker load < rivision-server-images.tar.gz

# 3. 验证镜像
docker images | grep rivision
# 应显示:
#   rivision-server   latest   ...
#   rivision-gateway   latest   ...

# 4. 配置
cd docker
cp .env.example .env && vim .env
# 必改:
#   SECRET_KEY=your-secret-key
#   REGISTRATION_TOKEN=your-token  (Node 注册时需要此 token)

# 5. 启动 (rivision-server + inference-gateway)
docker compose -f docker-compose.combined.k3.yml up -d

# 6. 验证
docker ps
# 应显示 2 个容器: rivision-server (:80), rivision-gateway (:8081)
curl http://localhost/health        # Server
curl http://localhost:8081/health   # Gateway
```

### 离线部署: Node K3 (每台)

```bash
# 1. 解压
tar xzf rivision_offline_deploy.tgz
cd rivision_offline/node

# 2. 加载镜像
docker load < rivision-node-image.tar.gz

# 3. 配置 (每台 K3 不同!)
cd docker
cp .env.example .env && vim .env
# 必改:
#   NODE_ID=k3-prod-01              (每台唯一)
#   GATEWAY_URL=http://SERVER_IP:8081  (Server K3 的 IP)
#   REGISTRATION_TOKEN=your-token   (与 Server 端一致)

# 4. 创建目录
mkdir -p ../logs

# 5. 启动
docker compose -f docker-compose.node.k3.yml up -d

# 6. 验证
docker logs -f rivision-node
# 应看到: 节点注册成功
curl http://localhost:9090/api/v1/health   # Node Agent
curl http://localhost:9080/health          # llama-server
```

### 连通性验证

```bash
# 在 Node K3 上验证能否连接 Gateway
curl http://SERVER_IP:8081/health

# 在 Server K3 上查看已注册节点
curl http://localhost:8081/api/v1/nodes
```

---

## 模型更新

```bash
# 添加新模型
docker compose -f docker/docker-compose.node.k3.yml down
cp new-model.gguf /opt/rivision_node/models/
vim /opt/rivision_node/config/models.conf    # 添加模型定义
docker compose -f docker/docker-compose.node.k3.yml up -d

# 切换模型
docker exec rivision-node bash /app/scripts/switch-model.sh <model-name>
docker compose -f docker/docker-compose.node.k3.yml restart
```

---

## 目录结构参考

```
/opt/rivision_node/                # 生产 K3 部署目录
├── docker/
│   ├── docker-compose.node.k3.yml
│   ├── .env.example
│   └── .env                       # ← 每台 K3 唯一配置
├── config/
│   ├── models.conf                # 模型定义
│   └── active-model.sh            # 当前活跃模型 (自动生成)
├── models/
│   ├── fastvlm-0.5b-q8_0.gguf
│   └── fastvlm-0.5b-mmproj-f16.gguf
└── logs/                          # 运行时日志
    ├── llama-20250305.log
    └── node-agent.log
```
