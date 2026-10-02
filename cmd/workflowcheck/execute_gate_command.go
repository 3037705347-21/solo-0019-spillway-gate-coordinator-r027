package main

import (
	"fmt"
	"net/http"
)

func runExecuteGateCommand(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-west", "西侧泄洪闸", 4, 0.25); err != nil {
		return err
	}
	if _, err := fixtureCalibration(client, "gate-west", 0, 4); err != nil {
		return err
	}
	command, err := fixtureCommand(client, "gate-west", 2.0, "平衡下游水位", "dispatcher-zhao")
	if err != nil {
		return err
	}
	commandID, err := stringField(command, "id")
	if err != nil {
		return err
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

	basis, err := objectField(executedCommand, "calibration_basis")
	if err != nil {
		return err
	}
	basisReference, err := stringField(basis, "reference")
	if err != nil {
		return err
	}
	if basisReference != "测量基准点 BM-2026-A" {
		return fmt.Errorf("calibration basis reference: expected %q, got %q", "测量基准点 BM-2026-A", basisReference)
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

	// A simulated fault aborts the command but must still retain the
	// calibration fact in force at execution time.
	if err := runFaultedCalibrationBasis(client); err != nil {
		return err
	}

	fmt.Println("execute-gate-command: command completed, faulted basis retained, token reuse rejected")
	return nil
}

func runFaultedCalibrationBasis(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-fault-cal", "故障校准闸", 4, 0.25); err != nil {
		return err
	}
	faultCalibration, err := fixtureCalibration(client, "gate-fault-cal", 0, 4)
	if err != nil {
		return err
	}
	faultCalID, err := stringField(faultCalibration, "id")
	if err != nil {
		return err
	}
	command, err := fixtureCommand(client, "gate-fault-cal", 2.0, "故障路径依据保留", "dispatcher-zhao")
	if err != nil {
		return err
	}
	commandID, err := stringField(command, "id")
	if err != nil {
		return err
	}
	approved, err := fixtureClearReview(client, commandID, "safety-he")
	if err != nil {
		return err
	}
	faultToken, err := stringField(approved, "execution_token")
	if err != nil {
		return err
	}

	faulted, err := client.post("/commands/"+commandID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          faultToken,
		"simulate_fault": true,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(faulted, http.StatusOK, "execute simulated fault"); err != nil {
		return err
	}
	faultedCommand, err := objectField(faulted.Body, "command")
	if err != nil {
		return err
	}
	faultedStatus, err := stringField(faultedCommand, "status")
	if err != nil {
		return err
	}
	if faultedStatus != "aborted" {
		return fmt.Errorf("fault execution status: expected aborted, got %q", faultedStatus)
	}
	basis, err := objectField(faultedCommand, "calibration_basis")
	if err != nil {
		return err
	}
	basisID, err := stringField(basis, "id")
	if err != nil {
		return err
	}
	if basisID != faultCalID {
		return fmt.Errorf("aborted command basis: expected %q, got %q", faultCalID, basisID)
	}
	faultedGate, err := objectField(faulted.Body, "gate")
	if err != nil {
		return err
	}
	faultedGateStatus, err := stringField(faultedGate, "status")
	if err != nil {
		return err
	}
	if faultedGateStatus != "faulted" {
		return fmt.Errorf("faulted gate status: expected faulted, got %q", faultedGateStatus)
	}
	return nil
}
