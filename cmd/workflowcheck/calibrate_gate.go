package main

import (
	"fmt"
	"net/http"
)

func runCalibrateGate(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-cal", "校准泄洪闸", 5, 0.25); err != nil {
		return err
	}

	// Register an already-expired measurement fact.
	expired, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "基准点 OLD-BM",
		"measured_aperture": 0.25,
		"min_aperture":      0,
		"max_aperture":      5,
		"valid_from":        "2020-01-01T00:00:00Z",
		"valid_until":       "2021-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
		"note":              "过期基准",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(expired, http.StatusCreated, "register expired calibration"); err != nil {
		return err
	}

	history, err := client.get("/gates/gate-cal/calibrations")
	if err != nil {
		return err
	}
	if err := requireStatus(history, http.StatusOK, "read calibration history"); err != nil {
		return err
	}
	if err := requireCalibrationCount(history, 1); err != nil {
		return err
	}

	first, err := fixtureCommand(client, "gate-cal", 2.5, "有效期外执行", "dispatcher-lin")
	if err != nil {
		return err
	}
	firstID, err := stringField(first, "id")
	if err != nil {
		return err
	}
	approvedFirst, err := fixtureClearReview(client, firstID, "safety-wu")
	if err != nil {
		return err
	}
	firstToken, err := stringField(approvedFirst, "execution_token")
	if err != nil {
		return err
	}
	expiredExecute, err := client.post("/commands/"+firstID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          firstToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(expiredExecute, http.StatusConflict, "reject execution against expired calibration"); err != nil {
		return err
	}
	if err := requireErrorCode(expiredExecute, "calibration_not_current"); err != nil {
		return err
	}

	// Recalibrate with a current, narrower trusted range. The approved
	// command stays reserved; the new fact supersedes the expired one.
	second, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "测量基准点 BM-2026-A",
		"measured_aperture": 0.25,
		"min_aperture":      0,
		"max_aperture":      3,
		"valid_from":        "2026-01-01T00:00:00Z",
		"valid_until":       "2027-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
		"note":              "重新校准",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(second, http.StatusCreated, "register replacement calibration"); err != nil {
		return err
	}
	secondCalibration, err := objectField(second.Body, "calibration")
	if err != nil {
		return err
	}
	secondCalID, err := stringField(secondCalibration, "id")
	if err != nil {
		return err
	}

	executed, err := client.post("/commands/"+firstID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          firstToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executed, http.StatusOK, "execute after recalibration"); err != nil {
		return err
	}
	executedCommand, err := objectField(executed.Body, "command")
	if err != nil {
		return err
	}
	basis, err := objectField(executedCommand, "calibration_basis")
	if err != nil {
		return err
	}
	basisID, err := stringField(basis, "id")
	if err != nil {
		return err
	}
	if basisID != secondCalID {
		return fmt.Errorf("execution basis: expected newest calibration %q, got %q", secondCalID, basisID)
	}

	// Historical replay of the terminal command stays a command-state
	// conflict regardless of the calibration timeline.
	replay, err := client.post("/commands/"+firstID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          firstToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(replay, http.StatusConflict, "reject replayed terminal command"); err != nil {
		return err
	}
	if err := requireErrorCode(replay, "command_not_executable"); err != nil {
		return err
	}

	// A new command whose target leaves the trusted range is refused at
	// execution time.
	outOfRange, err := fixtureCommand(client, "gate-cal", 4.5, "超出校准范围", "dispatcher-lin")
	if err != nil {
		return err
	}
	outOfRangeID, err := stringField(outOfRange, "id")
	if err != nil {
		return err
	}
	approvedOutOfRange, err := fixtureClearReview(client, outOfRangeID, "safety-wu")
	if err != nil {
		return err
	}
	outOfRangeToken, err := stringField(approvedOutOfRange, "execution_token")
	if err != nil {
		return err
	}
	outOfRangeExecute, err := client.post("/commands/"+outOfRangeID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          outOfRangeToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(outOfRangeExecute, http.StatusConflict, "reject movement outside calibration range"); err != nil {
		return err
	}
	if err := requireErrorCode(outOfRangeExecute, "aperture_outside_calibration"); err != nil {
		return err
	}

	// A third fact widens the range again.
	third, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "测量基准点 BM-2026-B",
		"measured_aperture": 2.5,
		"min_aperture":      0,
		"max_aperture":      5,
		"valid_from":        "2026-06-01T00:00:00Z",
		"valid_until":       "2028-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(third, http.StatusCreated, "register widened calibration"); err != nil {
		return err
	}

	// The already completed command still cites the second calibration.
	fetchedFirst, err := client.get("/commands/" + firstID)
	if err != nil {
		return err
	}
	fetchedCommand, err := objectField(fetchedFirst.Body, "command")
	if err != nil {
		return err
	}
	fetchedBasis, err := objectField(fetchedCommand, "calibration_basis")
	if err != nil {
		return err
	}
	fetchedBasisID, err := stringField(fetchedBasis, "id")
	if err != nil {
		return err
	}
	if fetchedBasisID != secondCalID {
		return fmt.Errorf("terminal command basis changed after recalibration: got %q", fetchedBasisID)
	}

	// Meanwhile the approved command now moves on the newest fact.
	widened, err := client.post("/commands/"+outOfRangeID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          outOfRangeToken,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(widened, http.StatusOK, "execute after widened calibration"); err != nil {
		return err
	}

	// Recalibration is allowed while a command occupies the gate and cannot
	// release the reservation; execution then uses the newest fact.
	if err := runReservedGateRecalibration(client); err != nil {
		return err
	}

	// A gate without any calibration refuses execution with a stable code.
	if err := runUnknownCalibration(client); err != nil {
		return err
	}

	// Stable validation failures.
	if err := runCalibrationValidationFailures(client); err != nil {
		return err
	}

	fmt.Println("calibrate-gate: calibration facts gate execution and historical evidence is preserved")
	return nil
}

func runReservedGateRecalibration(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-reserved-cal", "占用中校准闸", 4, 1); err != nil {
		return err
	}
	old, err := fixtureCalibration(client, "gate-reserved-cal", 0, 4)
	if err != nil {
		return err
	}
	oldID, err := stringField(old, "id")
	if err != nil {
		return err
	}
	command, err := fixtureCommand(client, "gate-reserved-cal", 2, "占用期间校准", "dispatcher-qin")
	if err != nil {
		return err
	}
	commandID, err := stringField(command, "id")
	if err != nil {
		return err
	}
	approved, err := fixtureClearReview(client, commandID, "safety-wu")
	if err != nil {
		return err
	}
	token, err := stringField(approved, "execution_token")
	if err != nil {
		return err
	}

	gateResult, err := client.get("/gates/gate-reserved-cal")
	if err != nil {
		return err
	}
	gate, err := objectField(gateResult.Body, "gate")
	if err != nil {
		return err
	}
	status, err := stringField(gate, "status")
	if err != nil {
		return err
	}
	activeCommandID, err := stringField(gate, "active_command_id")
	if err != nil {
		return err
	}
	if status != "reserved" || activeCommandID != commandID {
		return fmt.Errorf("gate should stay reserved during recalibration: status=%q", status)
	}

	newer, err := client.post("/gates/gate-reserved-cal/calibrations", map[string]any{
		"reference":         "占用期新基准",
		"measured_aperture": 1,
		"min_aperture":      0,
		"max_aperture":      4,
		"valid_from":        "2026-01-01T00:00:00Z",
		"valid_until":       "2027-06-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(newer, http.StatusCreated, "recalibrate reserved gate"); err != nil {
		return err
	}
	newerCalibration, err := objectField(newer.Body, "calibration")
	if err != nil {
		return err
	}
	newerID, err := stringField(newerCalibration, "id")
	if err != nil {
		return err
	}
	if newerID == oldID {
		return fmt.Errorf("recalibration reused the old calibration id %q", oldID)
	}

	executed, err := client.post("/commands/"+commandID+"/execute", map[string]any{
		"operator":       "operator-chen",
		"token":          token,
		"simulate_fault": false,
	})
	if err != nil {
		return err
	}
	if err := requireStatus(executed, http.StatusOK, "execute with calibration registered during reservation"); err != nil {
		return err
	}
	executedCommand, err := objectField(executed.Body, "command")
	if err != nil {
		return err
	}
	basis, err := objectField(executedCommand, "calibration_basis")
	if err != nil {
		return err
	}
	basisID, err := stringField(basis, "id")
	if err != nil {
		return err
	}
	if basisID != newerID {
		return fmt.Errorf("reserved-gate execution basis: expected %q, got %q", newerID, basisID)
	}
	return nil
}

func runUnknownCalibration(client *apiClient) error {
	if _, err := fixtureGate(client, "gate-bare", "无基准闸门", 4, 0.5); err != nil {
		return err
	}
	command, err := fixtureCommand(client, "gate-bare", 2, "无基准执行", "dispatcher-qin")
	if err != nil {
		return err
	}
	commandID, err := stringField(command, "id")
	if err != nil {
		return err
	}
	approved, err := fixtureClearReview(client, commandID, "safety-wu")
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
	if err := requireStatus(executed, http.StatusConflict, "reject execution without any calibration"); err != nil {
		return err
	}
	return requireErrorCode(executed, "calibration_unknown")
}

func runCalibrationValidationFailures(client *apiClient) error {
	missingGate, err := client.post("/gates/gate-missing/calibrations", map[string]any{
		"reference":         "基准点",
		"measured_aperture": 0.5,
		"min_aperture":      0,
		"max_aperture":      4,
		"valid_from":        "2026-01-01T00:00:00Z",
		"valid_until":       "2027-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(missingGate, http.StatusNotFound, "reject calibration for unknown gate"); err != nil {
		return err
	}

	badTimestamp, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "基准点",
		"measured_aperture": 0.5,
		"min_aperture":      0,
		"max_aperture":      4,
		"valid_from":        "not-a-timestamp",
		"valid_until":       "2027-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(badTimestamp, http.StatusBadRequest, "reject malformed valid_from"); err != nil {
		return err
	}

	beyondLimit, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "基准点",
		"measured_aperture": 0.5,
		"min_aperture":      0,
		"max_aperture":      99,
		"valid_from":        "2026-01-01T00:00:00Z",
		"valid_until":       "2027-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(beyondLimit, http.StatusBadRequest, "reject range beyond gate limit"); err != nil {
		return err
	}

	emptyReference, err := client.post("/gates/gate-cal/calibrations", map[string]any{
		"reference":         "   ",
		"measured_aperture": 0.5,
		"min_aperture":      0,
		"max_aperture":      4,
		"valid_from":        "2026-01-01T00:00:00Z",
		"valid_until":       "2027-01-01T00:00:00Z",
		"registered_by":     "maintainer-he",
	})
	if err != nil {
		return err
	}
	if err := requireStatus(emptyReference, http.StatusBadRequest, "reject empty measurement reference"); err != nil {
		return err
	}
	return nil
}

func requireCalibrationCount(result apiResult, expected int) error {
	items, ok := result.Body["calibrations"].([]any)
	if !ok {
		return fmt.Errorf("response is missing calibrations array: %s", prettyBody(result.Body))
	}
	if len(items) != expected {
		return fmt.Errorf("calibration history length: expected %d, got %d", expected, len(items))
	}
	return nil
}

func requireErrorCode(result apiResult, code string) error {
	actual, err := stringField(result.Body, "code")
	if err != nil {
		return fmt.Errorf("response is missing error code: %s", prettyBody(result.Body))
	}
	if actual != code {
		return fmt.Errorf("error code: expected %q, got %q", code, actual)
	}
	return nil
}
