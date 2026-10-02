package main

import (
	"fmt"
	"net/http"
	"time"
)

func runCalibrateGatePosition(client *apiClient) error {
	// 场景一：闸门从未登记校准（未知基准）时执行必须被拒绝。
	if _, err := fixtureGate(client, "gate-cal-a", "校准闸门甲", 5, 0.4); err != nil {
		return err
	}
	unknownCmd, err := fixtureCommand(client, "gate-cal-a", 2.0, "无基准执行", "dispatcher-qin")
	if err != nil {
		return err
	}
	unknownCmdID, err := stringField(unknownCmd, "id")
	if err != nil {
		return err
	}
	unknownApproved, err := fixtureClearReview(client, unknownCmdID, "safety-luo")
	if err != nil {
		return err
	}
	unknownToken, err := stringField(unknownApproved, "execution_token")
	if err != nil {
		return err
	}
	noBasis, err := client.post("/commands/"+unknownCmdID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          unknownToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(noBasis, http.StatusConflict, "reject execution without calibration"); err != nil {
		return err
	}
	if code, _ := stringField(noBasis.Body, "code"); code != "calibration_unknown" {
		return fmt.Errorf("unknown-basis error code: expected calibration_unknown, got %q", code)
	}

	// 占用中的闸门不能借重新校准绕过占用。
	busyCalibration, err := client.post("/gates/gate-cal-a/calibrations", map[string]any{
		"basis":             "坝顶 BM-X",
		"measured_aperture": 0.4,
		"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
		"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
		"recorded_by":       "maintainer-su",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(busyCalibration, http.StatusConflict, "reject recalibration of reserved gate"); err != nil {
		return err
	}
	gateAResult, err := client.get("/gates/gate-cal-a")
	if err != nil {
		return err
	}
	gateA, err := objectField(gateAResult.Body, "gate")
	if err != nil {
		return err
	}
	if status, _ := stringField(gateA, "status"); status != "reserved" {
		return fmt.Errorf("rejected recalibration must leave gate reserved, got %q", status)
	}
	if _, exists := gateA["current_calibration_id"]; exists {
		return fmt.Errorf("rejected recalibration must not attach a calibration reference")
	}
	emptyHistoryResult, err := client.get("/gates/gate-cal-a/calibrations")
	if err != nil {
		return err
	}
	if err := requireStatus(emptyHistoryResult, http.StatusOK, "read empty calibration history"); err != nil {
		return err
	}
	emptyHistory, err := calibrationsArray(emptyHistoryResult.Body)
	if err != nil {
		return err
	}
	if len(emptyHistory) != 0 {
		return fmt.Errorf("rejected calibration must not be stored, got %d entries", len(emptyHistory))
	}

	// 场景二：登记校准依据与有效范围，实时开度被修正为实测开度。
	if _, err := fixtureGate(client, "gate-cal-b", "校准闸门乙", 5, 0.4); err != nil {
		return err
	}
	firstCalibration, firstResponse, err := fixtureCalibration(client, "gate-cal-b", "坝顶基准点 BM-1", 0.42, time.Hour, "maintainer-su")
	if err != nil {
		return err
	}
	firstID, err := stringField(firstCalibration, "id")
	if err != nil {
		return err
	}
	if status, _ := stringField(firstCalibration, "status"); status != "active" {
		return fmt.Errorf("new calibration status: expected active, got %q", status)
	}
	if valid, _ := firstCalibration["currently_valid"].(bool); !valid {
		return fmt.Errorf("new calibration must be currently valid")
	}
	firstGate, err := objectField(firstResponse, "gate")
	if err != nil {
		return err
	}
	if aperture, err := floatField(firstGate, "aperture"); err != nil || aperture != 0.42 {
		return fmt.Errorf("calibration must correct live aperture to 0.42, got %v", aperture)
	}
	if currentID, _ := stringField(firstGate, "current_calibration_id"); currentID != firstID {
		return fmt.Errorf("gate must reference the new calibration, got %q", currentID)
	}
	embedded, err := objectField(firstGate, "current_calibration")
	if err != nil {
		return err
	}
	if basis, _ := stringField(embedded, "basis"); basis != "坝顶基准点 BM-1" {
		return fmt.Errorf("embedded calibration basis mismatch: %q", basis)
	}

	// 非法登记输入必须被稳定拒绝。
	for _, payload := range []map[string]any{
		{
			"basis":             "倒序有效期",
			"measured_aperture": 0.4,
			"valid_from":        time.Now().Format(time.RFC3339),
			"valid_until":       time.Now().Add(-time.Hour).Format(time.RFC3339),
			"recorded_by":       "maintainer-su",
		},
		{
			"basis":             "",
			"measured_aperture": 0.4,
			"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
			"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
			"recorded_by":       "maintainer-su",
		},
		{
			"basis":             "越界实测",
			"measured_aperture": 9,
			"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
			"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
			"recorded_by":       "maintainer-su",
		},
		{
			"basis":             "坏时间格式",
			"measured_aperture": 0.4,
			"valid_from":        "not-a-timestamp",
			"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
			"recorded_by":       "maintainer-su",
		},
	} {
		result, err := client.post("/gates/gate-cal-b/calibrations", payload)
		if err != nil {
			return err
		}
		if err := requireStatus(result, http.StatusBadRequest, "reject invalid calibration payload"); err != nil {
			return err
		}
	}

	// 场景三：重新校准原子取代旧依据，旧记录保留为 superseded。
	secondCalibration, secondResponse, err := fixtureCalibration(client, "gate-cal-b", "坝顶基准点 BM-2", 0.5, time.Hour, "maintainer-su")
	if err != nil {
		return err
	}
	secondID, err := stringField(secondCalibration, "id")
	if err != nil {
		return err
	}
	secondGate, err := objectField(secondResponse, "gate")
	if err != nil {
		return err
	}
	if aperture, err := floatField(secondGate, "aperture"); err != nil || aperture != 0.5 {
		return fmt.Errorf("recalibration must correct live aperture to 0.5, got %v", aperture)
	}
	if currentID, _ := stringField(secondGate, "current_calibration_id"); currentID != secondID {
		return fmt.Errorf("gate must reference the replacement calibration, got %q", currentID)
	}

	historyResult, err := client.get("/gates/gate-cal-b/calibrations")
	if err != nil {
		return err
	}
	if err := requireStatus(historyResult, http.StatusOK, "read calibration history"); err != nil {
		return err
	}
	history, err := calibrationsArray(historyResult.Body)
	if err != nil {
		return err
	}
	if len(history) != 2 {
		return fmt.Errorf("history must retain both calibrations, got %d", len(history))
	}
	if id, _ := stringField(history[0], "id"); id != secondID {
		return fmt.Errorf("history must be newest first")
	}
	if status, _ := stringField(history[1], "status"); status != "superseded" {
		return fmt.Errorf("previous calibration must be superseded, got %q", status)
	}
	if valid, _ := history[1]["currently_valid"].(bool); valid {
		return fmt.Errorf("superseded calibration must not read as currently valid")
	}
	if id, _ := stringField(history[1], "id"); id != firstID {
		return fmt.Errorf("superseded record must remain the original calibration")
	}

	// 场景四：执行基于新校准成功；之后再次重校准，已完成命令仍保留当时依据。
	cmdB, err := fixtureCommand(client, "gate-cal-b", 2.0, "校准后执行", "dispatcher-qin")
	if err != nil {
		return err
	}
	cmdBID, err := stringField(cmdB, "id")
	if err != nil {
		return err
	}
	if start, err := floatField(cmdB, "starting_aperture"); err != nil || start != 0.5 {
		return fmt.Errorf("command must start from recalibrated aperture 0.5, got %v", start)
	}
	approvedB, err := fixtureClearReview(client, cmdBID, "safety-luo")
	if err != nil {
		return err
	}
	tokenB, err := stringField(approvedB, "execution_token")
	if err != nil {
		return err
	}
	executedB, err := client.post("/commands/"+cmdBID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenB,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executedB, http.StatusOK, "execute on valid calibration"); err != nil {
		return err
	}
	executedBCommand, err := objectField(executedB.Body, "command")
	if err != nil {
		return err
	}
	basisB, err := objectField(executedBCommand, "calibration")
	if err != nil {
		return err
	}
	if id, _ := stringField(basisB, "calibration_id"); id != secondID {
		return fmt.Errorf("completed command must retain BM-2 basis, got %q", id)
	}

	if _, _, err := fixtureCalibration(client, "gate-cal-b", "坝顶基准点 BM-3", 2.02, time.Hour, "maintainer-su"); err != nil {
		return err
	}
	fetchedB, err := client.get("/commands/" + cmdBID)
	if err != nil {
		return err
	}
	storedB, err := objectField(fetchedB.Body, "command")
	if err != nil {
		return err
	}
	storedBasis, err := objectField(storedB, "calibration")
	if err != nil {
		return err
	}
	if id, _ := stringField(storedBasis, "calibration_id"); id != secondID {
		return fmt.Errorf("recalibration must not rewrite the completed command basis, got %q", id)
	}
	replayCompleted, err := client.post("/commands/"+cmdBID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenB,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(replayCompleted, http.StatusConflict, "replay completed command consistently"); err != nil {
		return err
	}
	if code, _ := stringField(replayCompleted.Body, "code"); code != "command_not_executable" {
		return fmt.Errorf("completed replay must return command_not_executable, got %q", code)
	}

	// 场景五：校准失效后执行被一致拒绝；失效期间占用仍受保护。
	if _, err := fixtureGate(client, "gate-cal-c", "校准闸门丙", 3, 0.2); err != nil {
		return err
	}
	if _, _, err := fixtureCalibration(client, "gate-cal-c", "临时基准点 BM-T", 0.2, 1500*time.Millisecond, "maintainer-su"); err != nil {
		return err
	}
	cmdC, err := fixtureCommand(client, "gate-cal-c", 1.0, "有效期内复核", "dispatcher-qin")
	if err != nil {
		return err
	}
	cmdCID, err := stringField(cmdC, "id")
	if err != nil {
		return err
	}
	approvedC, err := fixtureClearReview(client, cmdCID, "safety-luo")
	if err != nil {
		return err
	}
	tokenC, err := stringField(approvedC, "execution_token")
	if err != nil {
		return err
	}
	time.Sleep(1700 * time.Millisecond)
	expiredExecute, err := client.post("/commands/"+cmdCID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenC,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(expiredExecute, http.StatusConflict, "reject execution on expired calibration"); err != nil {
		return err
	}
	if code, _ := stringField(expiredExecute.Body, "code"); code != "calibration_expired" {
		return fmt.Errorf("expired error code: expected calibration_expired, got %q", code)
	}
	expiredRetry, err := client.post("/commands/"+cmdCID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenC,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(expiredRetry, http.StatusConflict, "repeat expired rejection consistently"); err != nil {
		return err
	}
	if code, _ := stringField(expiredRetry.Body, "code"); code != "calibration_expired" {
		return fmt.Errorf("repeated expired error must stay calibration_expired, got %q", code)
	}
	gateCResult, err := client.get("/gates/gate-cal-c")
	if err != nil {
		return err
	}
	gateC, err := objectField(gateCResult.Body, "gate")
	if err != nil {
		return err
	}
	if status, _ := stringField(gateC, "status"); status != "reserved" {
		return fmt.Errorf("expired rejection must leave gate reserved, got %q", status)
	}
	embeddedC, err := objectField(gateC, "current_calibration")
	if err != nil {
		return err
	}
	if valid, _ := embeddedC["currently_valid"].(bool); valid {
		return fmt.Errorf("expired calibration embedded in gate view must read invalid")
	}
	recalibrateBusy, err := client.post("/gates/gate-cal-c/calibrations", map[string]any{
		"basis":             "替换基准点",
		"measured_aperture": 0.2,
		"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
		"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
		"recorded_by":       "maintainer-su",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(recalibrateBusy, http.StatusConflict, "recalibration cannot bypass reservation after expiry"); err != nil {
		return err
	}

	// 场景六：故障闸门不能借重新校准清除故障；中止命令保留当时依据。
	if _, err := fixtureGate(client, "gate-cal-d", "校准闸门丁", 2, 0.2); err != nil {
		return err
	}
	faultCalibration, _, err := fixtureCalibration(client, "gate-cal-d", "故障前基准 BM-F", 0.2, time.Hour, "maintainer-su")
	if err != nil {
		return err
	}
	faultCalibrationID, err := stringField(faultCalibration, "id")
	if err != nil {
		return err
	}
	cmdD, err := fixtureCommand(client, "gate-cal-d", 1.0, "故障模拟", "dispatcher-qin")
	if err != nil {
		return err
	}
	cmdDID, err := stringField(cmdD, "id")
	if err != nil {
		return err
	}
	approvedD, err := fixtureClearReview(client, cmdDID, "safety-luo")
	if err != nil {
		return err
	}
	tokenD, err := stringField(approvedD, "execution_token")
	if err != nil {
		return err
	}
	faulted, err := client.post("/commands/"+cmdDID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenD,
		"simulate_fault": true,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(faulted, http.StatusOK, "execute with simulated fault"); err != nil {
		return err
	}
	faultCommand, err := objectField(faulted.Body, "command")
	if err != nil {
		return err
	}
	faultBasis, err := objectField(faultCommand, "calibration")
	if err != nil {
		return err
	}
	if id, _ := stringField(faultBasis, "calibration_id"); id != faultCalibrationID {
		return fmt.Errorf("aborted command must retain its calibration basis, got %q", id)
	}
	recalibrateFaulted, err := client.post("/gates/gate-cal-d/calibrations", map[string]any{
		"basis":             "故障后复测",
		"measured_aperture": 0.9,
		"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
		"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
		"recorded_by":       "maintainer-su",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(recalibrateFaulted, http.StatusConflict, "recalibration cannot clear a faulted gate"); err != nil {
		return err
	}
	// 已中止命令的复读必须稳定命中终态冲突，而不是受校准状态影响。
	replayAborted, err := client.post("/commands/"+cmdDID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          tokenD,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(replayAborted, http.StatusConflict, "replay aborted command consistently"); err != nil {
		return err
	}
	if code, _ := stringField(replayAborted.Body, "code"); code != "command_not_executable" {
		return fmt.Errorf("aborted replay must return command_not_executable, got %q", code)
	}

	// 场景七：未知闸门返回稳定 not_found。
	for _, result := range []func() (apiResult, error){
		func() (apiResult, error) {
			return client.post("/gates/gate-missing/calibrations", map[string]any{
				"basis":             "幽灵基准",
				"measured_aperture": 0.4,
				"valid_from":        time.Now().Add(-time.Minute).Format(time.RFC3339),
				"valid_until":       time.Now().Add(time.Hour).Format(time.RFC3339),
				"recorded_by":       "maintainer-su",
			})
		},
		func() (apiResult, error) { return client.get("/gates/gate-missing/calibrations") },
	} {
		outcome, err := result()
		if err != nil {
			return err
		}
		if err := requireStatus(outcome, http.StatusNotFound, "unknown gate calibration access"); err != nil {
			return err
		}
	}

	fmt.Println("calibrate-gate-position: calibration lifecycle and execution gating verified")
	return nil
}

func calibrationsArray(body map[string]any) ([]map[string]any, error) {
	value, exists := body["calibrations"]
	if !exists {
		return nil, fmt.Errorf("history response is missing calibrations field")
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("calibrations field is not an array")
	}
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("calibration entry is not an object")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
