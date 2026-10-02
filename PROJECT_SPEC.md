# 泄洪闸门操作协调服务

## 项目目标

该服务用于水工控制室协调泄洪闸门的命令申请、安全联锁复核和单次开度调整。系统在同一闸门上只允许一条已通过复核且尚未结束的执行命令，任何冲突都必须被显式拒绝，不能覆盖已经占用的闸门。

## 使用角色

- 运行协调员：登记闸门并提交开度调整命令。
- 安全复核员：提交联锁结论，决定命令是否通过。
- 闸门操作员：凭执行令牌启动命令并登记执行结果。

## 核心实体

- `gate`：闸门及其当前开度、运行状态、最大开度、活动命令和当前校准引用。
- `gate_command`：一次开度调整申请，包含目标开度、申请原因、状态、执行令牌和执行时冻结的校准依据。
- `interlock_decision`：安全复核结论，状态为 clear 或 blocked。
- `gate_calibration`：一次位置校准登记，包含测量依据、实测开度、有效期窗口、登记人和 active/superseded 状态。

## 业务流程

### register-gate

运行协调员通过 `POST /gates` 登记闸门。服务校验标识、名称、最大开度和初始开度，写入空闲状态；重复标识返回稳定冲突错误，不覆盖原记录。

### request-gate-command

运行协调员通过 `POST /commands` 提交开度调整命令。服务读取闸门当前开度和限制，校验目标开度与申请信息，创建待复核命令并冻结起始开度。

### review-safety-interlock

安全复核员通过 `POST /commands/{id}/interlock` 提交复核结论。clear 结论会原子占用闸门并签发一次执行令牌；blocked 结论会把命令置为 rejected。其他命令先占用同一闸门时，当前 clear 结论返回冲突且不改变闸门。

### execute-gate-command

闸门操作员通过 `POST /commands/{id}/execute` 提交执行令牌。服务先校验命令状态、令牌和闸门占用关系，再校验闸门当前校准事实，在临界区内完成 approved、executing、completed 或 aborted 的转换，并同步更新闸门状态。没有任何登记校准时返回 `calibration_unknown`；当前校准超出有效期时返回 `calibration_expired`，两种情况下命令保持 approved、闸门保持 reserved，不写入任何执行依据。命令进入 executing 时冻结校准依据快照；令牌在命令结束后失效，不能复用。

### calibrate-gate-position

维护人员通过 `POST /gates/{id}/calibrations` 登记测量依据（basis）、实测开度、有效期起止和登记人。服务校验闸门存在、字段与有效窗口（窗口在登记时刻必须开放），只在闸门 idle 时接受登记：reserved、moving 或 faulted 闸门一律冲突拒绝，重新校准不能绕过现有占用，也不能清除故障。登记原子地把实时开度修正为实测开度、把新校准置为闸门当前依据，并把此前的 active 校准置为 superseded；历史记录永不删除，可通过 `GET /gates/{id}/calibrations` 按时间倒序读取。

## 状态与规则

- `gate` 状态：idle、reserved、moving、faulted。
- `gate_command` 状态：pending、approved、rejected、executing、completed、aborted。
- `gate_calibration` 状态：active、superseded；是否当前有效由 active 状态与 `[valid_from, valid_until]` 窗口共同决定。
- 只有 pending 命令可以复核。
- 只有 approved 命令可以执行，且执行令牌必须匹配。
- 执行只接受闸门当前有效的校准事实：未知基准拒绝执行，校准过期拒绝执行；这两类拒绝不改变命令和闸门。
- 命令在 executing 时冻结校准依据；completed 或 aborted 命令保留当时的依据，后续重新校准或校准失效都不会改写它，终态命令复读始终得到同一个终态冲突。
- 只有 idle 闸门可以登记新校准；reserved、moving、faulted 闸门的登记请求冲突失败且不写入。
- 新校准原子取代旧 active 校准（旧记录置为 superseded）并修正实时开度；校准记录只追加、不删除。
- clear 复核与闸门占用必须同时成功；任一步失败时两者都不写入。
- blocked 复核使命令 rejected，不会占用闸门。
- 模拟故障会令命令 aborted、闸门 faulted，并保留最终开度。
- 已完成或已中止的命令不能再次执行。

## 模块边界

- `cmd/server`：进程入口、监听地址和退出控制。
- `internal/httpapi`：HTTP 路由、JSON 解码、状态码映射。
- `internal/coordination`：工作流编排、命令标识和执行令牌签发。
- `internal/domain`：实体、不变量和状态转换规则。
- `internal/storage`：带互斥保护的内存仓库和原子写入。
- `internal/contracts`：API 请求、响应和稳定错误结构。
- `internal/bootstrap`：运行时依赖装配。

## 接口

- `GET /health`
- `POST /gates`
- `GET /gates/{id}`
- `POST /gates/{id}/calibrations`
- `GET /gates/{id}/calibrations`
- `POST /commands`
- `GET /commands/{id}`
- `POST /commands/{id}/interlock`
- `POST /commands/{id}/execute`

错误响应统一为：

```json
{"code":"stable_error_code","message":"可读说明"}
```

## 验证计划

本初始化基线刻意不生成单元测试、夹具或浏览器测试文件，也不声明 `test_command`。后续任务阶段负责补充测试，并在该阶段建立红绿验证。当前每条工作流通过 `cmd/workflowcheck` 启动真实 HTTP 服务并调用公开接口，校验成功状态、持久化结果和关键失败分支。位置校准能力的工作流额外覆盖：未知基准拒绝执行、过期校准拒绝执行且占用保留、重新校准原子取代与历史保留、占用中与故障闸门拒绝重新校准、执行并发可串行化、终态命令复读结果一致且保留当时校准依据。
