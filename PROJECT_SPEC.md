# 泄洪闸门操作协调服务

## 项目目标

该服务用于水工控制室协调泄洪闸门的命令申请、安全联锁复核和单次开度调整。系统在同一闸门上只允许一条已通过复核且尚未结束的执行命令，任何冲突都必须被显式拒绝，不能覆盖已经占用的闸门。

## 使用角色

- 运行协调员：登记闸门并提交开度调整命令。
- 安全复核员：提交联锁结论，决定命令是否通过。
- 闸门操作员：凭执行令牌启动命令并登记执行结果。
- 维护人员：按测量基准登记闸门位置校准事实及其有效开度范围与有效期。

## 核心实体

- `gate`：闸门及其当前开度、运行状态、最大开度和活动命令。
- `gate_command`：一次开度调整申请，包含目标开度、申请原因、状态、执行令牌以及执行开始时冻结的校准依据。
- `interlock_decision`：安全复核结论，状态为 clear 或 blocked。
- `gate_calibration`：维护人员登记的位置校准事实，记录测量基准、测量开度、可信开度区间、有效期、登记人和登记时间；同一闸门最新登记的事实是唯一权威事实。

## 业务流程

### register-gate

运行协调员通过 `POST /gates` 登记闸门。服务校验标识、名称、最大开度和初始开度，写入空闲状态；重复标识返回稳定冲突错误，不覆盖原记录。

### request-gate-command

运行协调员通过 `POST /commands` 提交开度调整命令。服务读取闸门当前开度和限制，校验目标开度与申请信息，创建待复核命令并冻结起始开度。

### review-safety-interlock

安全复核员通过 `POST /commands/{id}/interlock` 提交复核结论。clear 结论会原子占用闸门并签发一次执行令牌；blocked 结论会把命令置为 rejected。其他命令先占用同一闸门时，当前 clear 结论返回冲突且不改变闸门。

### execute-gate-command

闸门操作员通过 `POST /commands/{id}/execute` 提交执行令牌。服务先校验命令状态与一次性令牌，再在同一临界区内选取闸门当前权威校准事实并校验其在执行时刻有效、且起止开度都落在可信开度区间内，随后完成 approved、executing、completed 或 aborted 的转换，并同步更新闸门状态。命令进入 executing 时冻结当时校准依据；令牌在命令结束后失效，不能复用。

### register-gate-calibration

维护人员通过 `POST /gates/{id}/calibrations` 登记校准事实，提交测量基准、测量开度、可信开度区间、有效期和登记人。服务校验闸门存在、区间位于闸门物理开度之内、测量开度位于区间之内、有效期起点早于终点。每次登记都会成为该闸门的权威事实，不覆盖或删除历史事实；登记操作不改变闸门状态和活动命令占用，因此不能绕过现有占用或终态。

`GET /gates/{id}/calibrations` 按登记顺序返回该闸门全部校准事实。

## 状态与规则

- `gate` 状态：idle、reserved、moving、faulted。
- `gate_command` 状态：pending、approved、rejected、executing、completed、aborted。
- 只有 pending 命令可以复核。
- 只有 approved 命令可以执行，且执行令牌必须匹配。
- 命令执行必须基于该闸门当前权威校准事实：无任何事实返回 `calibration_unknown`，事实未生效或已过期返回 `calibration_not_current`，起止开度超出可信区间返回 `aperture_outside_calibration`。
- 校准事实的有效期为半开区间 [valid_from, valid_until)，到期时刻起不再可信。
- 命令进入 executing 时冻结校准依据；completed 与 aborted 命令永久保留该依据，后续重新校准不回写历史命令。
- 重新校准只追加事实，不改闸门状态或活动命令引用，不能释放占用、中止执行或绕过终态。
- 已完成或已中止命令的重复执行一律返回命令状态冲突，与校准变化无关。
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
- `POST /commands`
- `GET /commands/{id}`
- `POST /commands/{id}/interlock`
- `POST /commands/{id}/execute`
- `POST /gates/{id}/calibrations`
- `GET /gates/{id}/calibrations`

错误响应统一为：

```json
{"code":"stable_error_code","message":"可读说明"}
```

## 验证计划

当前每条工作流通过 `cmd/workflowcheck` 启动真实 HTTP 服务并调用公开接口，校验成功状态、持久化结果和关键失败分支。`calibrate-gate` 工作流覆盖过期校准拒绝、重新校准后执行、历史命令复读、可信区间越界、占用期间重新校准、未知基准和稳定校验错误；领域与仓库层另有单元测试覆盖有效期边界、依据冻结和重新校准与执行并发。
