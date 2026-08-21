# task145-orbitops Benzhi 评测说明

## 业务问题

本镜像解决在轨卫星的运行计划与测控预报问题：把每颗卫星的最新轨道根数（或两行轨道根数 TLE）作为权威输入，推算未来一段时间内的星历、地面站过境窗口、轨道保持机动与碰撞规避机动，并把过境窗口、机动指令与碰撞事件持久化到 SQLite，进程重启后能从轨道根数与事件历史重建过境时间表与下一保持窗口。所有航天动力学计算（开普勒方程、J2 摄动、大气阻力衰减、地面站可见性与过境窗口、轨道保持 Δv、碰撞预警与规避）均由 Go 后端完成；前端只展示后端计算结果。

主要输入：轨道根数（a/e/i/Ω/ω/M + 历元）、地面站（经纬度/海拔/最小仰角）、预报起止与步长、机动类型与执行时刻、双星会合评估请求。

主要输出：星历（ECI + 大地经纬高）、过境窗口（AOS/LOS/TCA/峰值仰角/方位角/光照）、机动指令（Δv、目标根数）、碰撞告警（TCA/最小距离/概率）、重启重建的「下一过境/保持窗口/未关闭告警数」缓存。

## 标准本地命令

```bash
go build ./...              # 编译
go run . --addr=:8080 --db=orbitops.db   # 启动 HTTP 服务（前端 http://localhost:8080/）
go test ./...               # 单元测试
go run . --migrate-only     # 只建表退出
go run . --smoke-test       # 自检：录卫星/站→预报过境→页面+API→ReconcileAll 比对重启前后下一窗口逐字段相等后自行退出
```

## Docker 构建

`build_benzhi_docker.sh` 两个参数：镜像名、平台。双架构构建命令：

```bash
bash ./build_benzhi_docker.sh go-task-benzhi:amd64 linux/amd64
bash ./build_benzhi_docker.sh go-task-benzhi:arm64 linux/arm64
```

进入容器：

```bash
docker run -it go-task-benzhi:amd64
```

容器内可运行（依赖已在镜像内）：

```bash
cd /app && go run . --smoke-test     # 自检退出
go run . --addr=:8080 --db=/tmp/orbitops.db   # 启动服务（另开 shell 用 curl 访问）
```

## 前端

- 技术栈：原生 HTML/CSS/JS，无构建工具、无 Node、无 npm；经 `//go:embed web` 打入 Go 二进制。
- 前端目录：`internal/webfs/web/`（index.html / style.css / app.js）。
- 构建方式：无前端构建步骤；`go build ./...` 经 `//go:embed web` 把页面打进二进制。
- 构建产物路径：二进制内嵌（无独立产物目录）。
- 页面 URL：`http://localhost:8080/`（服务启动后）。
- 页面覆盖真实读写流程：录卫星→录地面站→预报过境→看过境时间表→触发轨道保持机动→执行 ReconcileAll 看重启重建结果→看星座摘要。

## 页面与业务 API 的 smoke-test

`--smoke-test` 会实际请求页面 `/` 与业务 API（`/api/satellites`、`/api/groundstations`、`/api/contacts/forecast`、`/api/maneuvers`、`/api/recompute`），并比对 ReconcileAll 前后「下一窗口」快照逐字段相等（幂等），执行后自行退出。镜像内验证：

```bash
docker run --rm go-task-benzhi:amd64 bash -c 'cd /app && CGO_ENABLED=0 GOTOOLCHAIN=local go run . --smoke-test'
```
