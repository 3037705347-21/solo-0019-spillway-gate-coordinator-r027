# 泄洪闸门操作协调服务

这是一个无外部依赖的 Go 后端，用本地内存仓库协调闸门登记、开度调整命令、安全联锁复核、执行结果和位置校准。服务强调单闸门活动命令互斥、原子状态转换、执行令牌生命周期、校准事实有效期和稳定错误传播。

## 运行

```sh
go build ./...
go run ./cmd/server --addr 127.0.0.1:18119
```

健康入口为 `GET http://127.0.0.1:18119/health`。

## API

```text
POST /gates
GET  /gates/{id}
POST /commands
GET  /commands/{id}
POST /commands/{id}/interlock
POST /commands/{id}/execute
POST /gates/{id}/calibrations
GET  /gates/{id}/calibrations
```

登记闸门示例：

```json
{
  "id": "gate-east",
  "name": "东侧泄洪闸",
  "max_aperture": 4.5,
  "initial_aperture": 0.5
}
```

提交命令示例：

```json
{
  "gate_id": "gate-east",
  "target_aperture": 2.25,
  "reason": "上游水位调整",
  "requested_by": "dispatcher-lin"
}
```

联锁复核示例：

```json
{
  "verdict": "clear",
  "verifier": "safety-wu",
  "note": "上下游条件满足"
}
```

执行示例：

```json
{
  "operator": "operator-chen",
  "token": "复核响应中的 execution_token",
  "simulate_fault": false
}
```

命令执行只接受闸门当前有效的校准事实。维护人员先登记测量基准、可信开度区间和有效期：

```json
{
  "reference": "测量基准点 BM-2026-A",
  "measured_aperture": 0.5,
  "min_aperture": 0,
  "max_aperture": 4.5,
  "valid_from": "2026-01-01T00:00:00Z",
  "valid_until": "2027-01-01T00:00:00Z",
  "registered_by": "maintainer-he",
  "note": "季度测量后登记"
}
```

- 没有任何校准事实时执行返回 `409 calibration_unknown`。
- 最新事实未生效或已过期时执行返回 `409 calibration_not_current`（有效期为半开区间，到期时刻即失效）。
- 起始或目标开度超出可信区间时执行返回 `409 aperture_outside_calibration`。
- 重新校准只追加新事实，不解除闸门占用、不改变终态命令；命令开始执行时冻结当时依据，completed/aborted 后永久保留。

## 目录

- `cmd/server`：服务进程。
- `cmd/workflowcheck`：四条工作流的真实 HTTP 检查入口。
- `internal/bootstrap`：依赖装配。
- `internal/contracts`：公开数据契约。
- `internal/coordination`：业务工作流。
- `internal/domain`：实体、不变量和状态机。
- `internal/httpapi`：路由、解码和错误映射。
- `internal/storage`：互斥保护的内存仓库。

## 工作流检查

```sh
go run ./cmd/workflowcheck --workflow register-gate
go run ./cmd/workflowcheck --workflow request-gate-command
go run ./cmd/workflowcheck --workflow review-safety-interlock
go run ./cmd/workflowcheck --workflow execute-gate-command
go run ./cmd/workflowcheck --workflow calibrate-gate
```

这些检查会创建临时 HTTP 服务并调用真实路由。它们不调用外部网络，也不依赖数据库。

## 测试

领域层与仓库层提供自动化测试，覆盖校准有效期边界、可信区间、依据冻结，以及重新校准与执行并发时的单一一致结果：

```sh
go test ./...
```
