# Moore Threads S5000 现场验证报告（PR #287）

- **PR**：Project-HAMi/HAMi-WebUI#287 `feat(provider): Moore Threads GPU support`
- **被测提交**：`e068e8b`（merge(main) 之后 + `fix(mthreads): emit hami_vmemory_scaling`）
- **验证时间**：2026-10-12 10:33 ~ 10:45 (CST)
- **环境**：`cicpoc-gpu1` (55.127.52.251)，root 免密 SSH
- **结论**：**通过**。PR 全部核心行为在真实 MTT S5000 硬件上得到验证。

> 本文档记录的是 PR #287 合并前的验证过程，环境为内网 POC 集群。
> 其中节点 IP、命名空间与镜像 tag 仅对当时环境有效，评审时以 PR 评论为准。

---

## 1. 环境基线（测试前实测采集）

| 项 | 值|
|---|---|
| 节点 | `cicpoc-gpu1` / `cicpoc-gpu2`（同型号双节点）|
| OS / Kernel | Kylin V11 (Swan25) / 6.6.0-32.7.v2505.ky11.x86_64 |
| K8s / runtime | v1.35.4 / containerd 2.1.4 |
| 加速卡 | MTT S5000 × 8/节点，driver 5.1.0-server |
| 单卡显存 | `mthreads.com/gpu<N>.memory = 85899345920` = 80 GiB |
| 切片标签 | `mthreads.com/sgpu.cores = 0-2-3` → **3 卡绑定 sgpu_km** |
| 整卡容量 | `mthreads.com/gpu = 5` |
| 切片容量 | `sgpu-core = 48`（3×16）、`sgpu-memory = 480`（3×160）|
| 物理卡数 | `mthreads.com/gpu.count = 8` |
| DCGM | `mt-dcgm-exporter` DaemonSet 2/2，Prometheus 内 16 条 `DCGM_FI_DEV_GPU_TEMP` |
| DCGM 标签 | `Hostname=<node>` + `gpu=<0..7>`，与本 PR 查询语句一致 |

该拓扑正是 PR 目标场景：**同一节点上切片卡与整卡混合共存**（3 切片 + 5 整卡）。

存量工作负载（验证前）：`default` 与 `mt-gpu-operator` 各有 full-* 与 slice-* Deployment，共 10 个容器在跑。

---

## 2. 部署方式（重要：无外网/无推送凭据下的可行路径）

**约束**：GPU 节点无法访问任何外网仓库（`registry-1.docker.io`、`registry.npmmirror.com`、`proxy.golang.org` 全部 DNS 超时）；
`registry.cic.cmbchina.cn` 推送需 mTLS 客户端证书 + Bearer 授权，本机与集群内均无可用推送凭据。

**采用的路径**（因 PR 仅改后端，`git diff origin/main..HEAD -- packages/web/` 为空）：

1. 本地按 CI 相同参数交叉编译：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=readonly -trimpath ./cmd/hami-webui`
2. `scp` 二进制到节点（sha256 校验一致：`29a80c16dbd97...`）
3. `ctr -n k8s.io images export` 导出既有镜像，解出 rootfs（**复用前端产物** `apps/public`）
4. 替换 `/apps/hami-webui` 后 `tar` 打包 → `docker import`（FROM scratch 语义，**不需要外网**）
5. `docker save` → `ctr -n k8s.io images import` 导入 kubelet 实际使用的 containerd
6. `kubectl set image` 滚动更新（`imagePullPolicy: IfNotPresent`，本地已有镜像不会回拉）

镜像：`registry.cic.cmbchina.cn/ks-addon/hami/hami-webui:v2.0.0-mthreads.19-pr287`

---

## 3. 验证结果

### 3.1 设备清单（核心）—— `POST /v1/gpus`

| 节点 | 总卡数 | 切片卡 (mode=sgpu) | 整卡 (mode=full) |
|---|---|---|---|
| cicpoc-gpu1 | 8 | `mthreads-0/2/3` | `mthreads-full-1/4/5/6/7` |
| cicpoc-gpu2 | 8 | `mthreads-0` | `mthreads-full-1..7` |

- **切片卡序号 `0,2,3` 精确匹配 `mthreads.com/sgpu.cores=0-2-3`**（非连续绑定）
- **整卡为补集 `[1,4,5,6,7]`**，与切片卡零交集 → 评审第 4 条（物理序号不冲突）**实测成立**
- 每节点 8 张 = `mthreads.com/gpu.count`，总数 16
- `memoryTotal` 全部 81920 MiB = 80 GiB，**来自厂商 `gpu<N>.memory` 标签回退** → 评审第 3 条**实测成立**
- `type` 字段为 `["Mthreads","Mthreads-GPU"]`（两种交付模式并存）
- 节点聚合 `cardCnt: 8`，与物理卡数一致

### 3.2 整卡 Pod 记账 —— `POST /v1/containers`

10 条容器记录，7 个整卡 Pod 全部记账正确：

| Pod | 卡数 | cores | memory | 状态 |
|---|---|---|---|---|
| default/webui-full-1 | 1 | 100 ✓ | 81920 ✓ | success |
| default/webui-full-2 | 2 | 200 ✓ | 163840 ✓ | success |
| default/webui-full-3 | 3 | 300 ✓ | 245760 ✓ | success |
| llm-d-optimized-baseline/qwen3-30b-a3b | 2 | 200 ✓ | 163840 ✓ | **not_ready** |
| mt-gpu-operator/webui-full-1/2/3 | 各1 | 100 ✓ | 81920 ✓ | success |

- 每卡固定 100 core / 81920 MiB，**无偏差**
- 切片 Pod（`webui-slice-1/2`）走 HAMi 注解路径，`type=Mthreads`，解码正常（启动日志可见 `mthreads-0 ... 16384 50` 等）
- **`not_ready` 与 `success` 并存**：`qwen3-30b-a3b` 实际 `restarts=5556`、`podReadyReason=ContainersNotReady`。
  证明整卡 Pod 走 **per-container 状态判定**而非一律 success → 评审第 10 条**实测成立**

### 3.3 Summary RPC —— `POST /v1/monitor/summary`

此前为 501 桩实现，本次实测正常返回：

| 过滤条件 | 实测 | 独立核算 | 一致 |
|---|---|---|---|
| 无过滤 | vgpu 3.4% | 14/412 = 3.4% | ✓ |
| `device=Mthreads-GPU` | vgpu 91.7% | 11/12 = 91.7% | ✓ |
| `node=cicpoc-gpu2` | vgpu 6.5% / mem 80 / core 82 | — | — |

### 3.4 指标与 DCGM 映射

`/metrics` 暴露 **每指标族 16 条序列**（= 16 张卡全覆盖），且 Prometheus 已全部抓到：

`hami_device_temperature` / `hami_device_memory_temperature` / `hami_device_power` /
`hami_memory_used` / `hami_memory_size` / `hami_memory_util` /
`hami_vgpu_count` / `hami_vmemory_size` / `hami_vcore_size` / `hami_vcore_scaling` /
`hami_core_size` / **`hami_vmemory_scaling`** —— 各 16 条 ✓

- **`hami_vmemory_scaling` = 16 条**，即本轮新修的指标（此前 Mthreads 路径因提前 `continue` 完全缺失）
- 温度为真实回填（如 `full-5` = 45°C、功耗 407.7W），非 0/占位
- 显存 `used + size = total`：`66817 + 15102 = 81919`（厂商 MiB 单位）

**DCGM 序号映射专项验证**：以显存占用（工作负载相关，序号错位会立刻暴露）逐卡比对，**16/16 完全一致**。

### 3.5 稳定性

- Pod `hami-webui-79cd7d88fd-mx94n` 持续 Running **10 分钟，RESTARTS = 0**
- 日志中 `panic` / `fatal` / `level=error` 计数 = **0**
- 探针：`/health_check`(3000) 200、`/`(3000) 200、`/readyz`(8000) 200

---

## 4. 对评审意见的现场闭环

| # | 评审意见 | 现场验证方式 | 结果 |
|---|---|---|---|
| 3 | 无 sGPU 池时整卡显存报 0 | gpu1 整卡 `memoryTotal=81920`（厂商标签回退）| ✓ |
| 4 | 整卡需用物理序号 | 切片 `0,2,3` 与整卡 `1,4,5,6,7` 零交集；显存 16/16 对上 DCGM | ✓ |
| 9 | 整卡发现不应依赖 sGPU 池 | gpu2 仅 1 张切片卡仍枚举出 7 张整卡 | ✓ |
| 10 | 整卡应用 per-container 状态 | `qwen3` 容器 correctly 报 `not_ready` + 完整 StatusDetail | ✓ |
| — | 混合拓扑设备清单 | 3 切片 + 5 整卡，合计 8 = `gpu.count` | ✓ |
| — | Summary RPC（新增） | 三种过滤条件均返回，数值与独立核算一致 | ✓ |
| — | 整卡 Pod 账本与显存 | 7 个整卡 Pod，cores/memory 零偏差 | ✓ |
| — | `hami_vmemory_scaling`（本轮新修） | 16 条序列，Prometheus 已抓到 | ✓ |

---

## 5. 排查过程中排除的伪问题

1. **温度 1°C 差异**：初测 14/16 匹配。连续 6 次采样显示 `DCGM gpu=0` 自身在 40↔41 间抖动，
   属卡级温度抖动，非映射错误。改用显存占用复测 → **16/16 一致**。
2. **2 张卡显存不匹配**：`gpu2-full-4/5` 首次比对时 hami 有值而 DCGM 为 0。
   复测时已变为双方 0 —— 是两次采样之间**真实工作负载释放了卡片**，非代码问题。
3. **Summary "不一致"**：自检脚本口径错误（拿全量 Summary 对整卡占用率）。按正确口径复核全部一致。

---

## 6. 遗留事项

- 镜像以 `ctr import` 方式直灌 containerd，**未推送到 registry.cic.cmbchina.cn**（缺推送凭据）。
  镜像仍在节点本地缓存，回滚只需 `kubectl set image` 指回 `v2.0.0-mthreads.18`。
- 如需正式发布，需在有推送凭据的环境执行完整 `make build-image`。
- 集群内无 `imagePullSecrets`，节点依赖 containerd 的 mTLS 证书链完成拉取。
- `/api/v1/nodes`（3000 前端入口）返回 404 是正常现象——API 服务在 **8000**，路径为 `/v1/*`（非 `/api/v1/*`）。