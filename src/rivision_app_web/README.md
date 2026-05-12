# RiVision App Web

独立的RiVision前端应用，支持第三方集成和定制化开发。

## 架构设计

```
rivision-k3-deploy/src/
├── rivision_hub/           # 纯后端API服务
│   └── internal/api/       # REST API
│
├── rivision_app_web/       # ✅ 独立前端项目
│   ├── src/
│   │   ├── views/          # 页面视图
│   │   ├── components/     # 通用组件
│   │   ├── api/            # API客户端层
│   │   └── stores/         # Pinia状态管理
│   └── public/
│
└── rivision_worker/        # Worker节点
```

## 优势

| 特性 | 说明 |
|-----|------|
| **解耦部署** | 前后端独立部署，支持CDN分发 |
| **第三方集成** | 可嵌入到其他系统或替换为自定义UI |
| **技术栈灵活** | 前端可独立升级Vue/React等 |
| **多租户支持** | 不同租户可使用不同的前端配置 |

## 快速开始

```bash
# 安装依赖
npm install

# 开发模式 (默认代理到 http://localhost:8080)
npm run dev

# 构建生产版本
npm run build

# 预览构建结果
npm run preview
```

## 环境配置

创建 `.env.local` 文件:

```env
# Hub API 地址
VITE_API_BASE_URL=http://your-hub-server:8080

# WebSocket 地址
VITE_WS_BASE_URL=ws://your-hub-server:8080
```

## API客户端使用

```typescript
import { searchApi, cameraApi, nodeApi } from '@/api/client'

// 文本搜索
const results = await searchApi.searchByText('红色衣服的人')

// 获取摄像头列表
const cameras = await cameraApi.list()

// 获取节点状态
const nodes = await nodeApi.list()
```

## 第三方集成

### 方式1: 嵌入iframe

```html
<iframe src="https://your-rivision-web.com/search" />
```

### 方式2: API直接调用

```javascript
// 使用独立的API客户端
import { createApiClient } from 'rivision-app-web/api/client'

const api = createApiClient({
  baseURL: 'https://your-hub.com/api/v1',
  token: 'your-token'
})
```

### 方式3: 组件库复用

```javascript
// 导入搜索组件
import SearchView from 'rivision-app-web/views/SearchView.vue'
```

## 目录结构

```
src/
├── api/
│   └── client.ts       # API客户端封装
├── views/
│   ├── DashboardView.vue   # 仪表盘
│   └── SearchView.vue      # 智能搜索
├── components/
│   ├── CameraGrid.vue      # 摄像头网格
│   ├── AlertList.vue       # 告警列表
│   └── SearchInput.vue     # 搜索输入
├── stores/
│   ├── auth.ts         # 认证状态
│   └── camera.ts       # 摄像头状态
└── router/
    └── index.ts        # 路由配置
```

## 与rivision_hub的关系

| 组件 | rivision_hub | rivision_app_web |
|-----|-------------|------------------|
| API | ✅ 提供REST API | 调用API |
| 认证 | ✅ Token验证 | 存储Token |
| WebSocket | ✅ 推送事件 | 接收展示 |
| 静态资源 | ❌ 不包含 | ✅ 独立托管 |
