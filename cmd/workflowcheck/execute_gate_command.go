package main

import (
	"fmt"
	"net/http"
	"time"
)

func runExecuteGateCommand(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-west", "西侧泄洪闸", 4, 0.25); err != nil {
		return err
	}
	calibration, _, err := fixtureCalibration(client, "gate-west", "坝顶水准点 BM-07 复测", 0.3, time.Hour, "maintainer-su")
	if err != nil {
		return err
	}
	calibrationID, err := stringField(calibration, "id")
	if err != nil {
		return err
	}
	// 登记校准会把实时开度修正为实测开度，命令必须基于修正后的事实。
	command, err := fixtureCommand(client, "gate-west", 2.0, "平衡下游水位", "dispatcher-zhao")
	if err != nil {
		return err
	}
	commandID, err := stringField(command, "id")
	if err != nil {
		return err
	}
	startingAperture, err := floatField(command, "starting_aperture")
	if err != nil {
		return err
	}
	if startingAperture != 0.3 {
		return fmt.Errorf("starting aperture: expected calibrated 0.3, got %v", startingAperture)
	}

	approved, err := fixtureClearReview(client, commandID, "safety-he")
	if err != nil {
		return err
	}
	token, err := stringField(approved, "execution_token")
	if err != nil {
		return err
	}

	executed, err := client.post("/commands/"+commandID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          token,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executed, http.StatusOK, "execute approved command"); err != nil {
		return err
	}
	executedCommand, err := objectField(executed.Body, "command")
	if err != nil {
		return err
	}
	completedStatus, err := stringField(executedCommand, "status")
	if err != nil {
		return err
	}
	if completedStatus != "completed" {
		return fmt.Errorf("execution status: expected completed, got %q", completedStatus)
	}
	basis, err := objectField(executedCommand, "calibration")
	if err != nil {
		return err
	}
	basisID, err := stringField(basis, "calibration_id")
	if err != nil {
		return err
	}
	if basisID != calibrationID {
		return fmt.Errorf("retained calibration basis: expected %q, got %q", calibrationID, basisID)
	}

	executedGate, err := objectField(executed.Body, "gate")
	if err != nil {
		return err
	}
	gateStatus, err := stringField(executedGate, "status")
	if err != nil {
		return err
	}
	aperture, err := floatField(executedGate, "aperture")
	if err != nil {
		return err
	}
	if gateStatus != "idle" || aperture != 2.0 {
		return fmt.Errorf("settled gate mismatch: status=%q aperture=%v", gateStatus, aperture)
	}

	reused, err := client.post("/commands/"+commandID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          token,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(reused, http.StatusConflict, "reject reused execution token"); err != nil {
		return err
	}

	// 复读已完成命令时，命令仍保留执行当时的校准依据。
	fetched, err := client.get("/commands/" + commandID)
	if err != nil {
		return err
	}
	if err := requireStatus(fetched, http.StatusOK, "read completed command"); err != nil {
		return err
	}
	storedCommand, err := objectField(fetched.Body, "command")
	if err != nil {
		return err
	}
	storedBasis, err := objectField(storedCommand, "calibration")
	if err != nil {
		return err
	}
	storedBasisID, err := stringField(storedBasis, "calibration_id")
	if err != nil {
		return err
	}
	if storedBasisID != calibrationID {
		return fmt.Errorf("stored command calibration basis: expected %q, got %q", calibrationID, storedBasisID)
	}

	fmt.Println("execute-gate-command: command executed on a valid calibration and basis retained")
	return nil
}
