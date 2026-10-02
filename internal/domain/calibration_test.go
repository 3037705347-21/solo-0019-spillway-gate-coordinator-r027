package domain

import (
	"errors"
	"testing"
	"time"
)

func testTime(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func validCalibration(now time.Time) Calibration {
	calibration, err := NewCalibration(
		"cal-1",
		"gate-1",
		"BM-A",
		1.0,
		0,
		4,
		5,
		testTime(2026, time.January, 1),
		testTime(2027, time.January, 1),
		"maintainer-he",
		"",
		now,
	)
	if err != nil {
		panic(err)
	}
	return calibration
}

func TestNewCalibrationRejectsInvalidInput(t *testing.T) {
	now := testTime(2026, time.January, 1)

	_, err := NewCalibration("", "gate-1", "BM", 1, 0, 4, 5,
		testTime(2026, time.January, 1), testTime(2027, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_id" {
		t.Fatalf("missing id: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", " ", 1, 0, 4, 5,
		testTime(2026, time.January, 1), testTime(2027, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_reference" {
		t.Fatalf("blank reference: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", "BM", 1, 3, 2, 5,
		testTime(2026, time.January, 1), testTime(2027, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_range" {
		t.Fatalf("inverted range: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", "BM", 1, 0, 6, 5,
		testTime(2026, time.January, 1), testTime(2027, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_range" {
		t.Fatalf("range beyond gate limit: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", "BM", 5, 0, 4, 5,
		testTime(2026, time.January, 1), testTime(2027, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_measured_aperture" {
		t.Fatalf("measured aperture outside range: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", "BM", 1, 0, 4, 5,
		testTime(2027, time.January, 1), testTime(2026, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_validity" {
		t.Fatalf("inverted validity window: got %v", err)
	}

	_, err = NewCalibration("cal-1", "gate-1", "BM", 1, 0, 4, 5,
		testTime(2026, time.January, 1), testTime(2026, time.January, 1), "m", "", now)
	if err == nil || ErrorCodeOf(err) != "invalid_calibration_validity" {
		t.Fatalf("zero-length validity window: got %v", err)
	}
}

func TestCalibrationGuardExecution(t *testing.T) {
	now := testTime(2026, time.January, 1)
	calibration := validCalibration(now)

	if err := calibration.GuardExecution(0.5, 2.5, testTime(2026, time.June, 1)); err != nil {
		t.Fatalf("in-range current movement rejected: %v", err)
	}

	err := calibration.GuardExecution(0.5, 2.5, testTime(2025, time.December, 31))
	if err == nil || ErrorCodeOf(err) != "calibration_not_current" {
		t.Fatalf("not-yet-valid calibration: got %v", err)
	}

	// Half-open window: the exact expiry instant is no longer current.
	err = calibration.GuardExecution(0.5, 2.5, testTime(2027, time.January, 1))
	if err == nil || ErrorCodeOf(err) != "calibration_not_current" {
		t.Fatalf("expiry instant treated as current: got %v", err)
	}

	err = calibration.GuardExecution(0.5, 4.5, testTime(2026, time.June, 1))
	if err == nil || ErrorCodeOf(err) != "aperture_outside_calibration" {
		t.Fatalf("target above range: got %v", err)
	}

	err = calibration.GuardExecution(4.5, 2.0, testTime(2026, time.June, 1))
	if err == nil || ErrorCodeOf(err) != "aperture_outside_calibration" {
		t.Fatalf("start above range: got %v", err)
	}
}

func TestSnapshotFreezesCalibrationFact(t *testing.T) {
	now := testTime(2026, time.January, 1)
	calibration := validCalibration(now)
	basis := calibration.Snapshot()
	if basis.CalibrationID != calibration.ID || basis.Reference != calibration.Reference {
		t.Fatalf("snapshot lost calibration identity: %+v", basis)
	}
	if basis.ValidUntil != calibration.ValidUntil {
		t.Fatalf("snapshot lost validity window: %+v", basis)
	}
}

func approvedCommandWithToken(t *testing.T) (GateCommand, string) {
	t.Helper()
	command, err := NewGateCommand("cmd-1", "gate-1", 2.0, 1.0, 4, "reason", "dispatcher", testTime(2026, time.January, 1))
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	token := "token-1"
	if err := command.Review(
		InterlockDecision{Verdict: InterlockClear, Verifier: "safety", ReviewedAt: testTime(2026, time.January, 2)},
		token,
		testTime(2026, time.January, 2),
	); err != nil {
		t.Fatalf("review: %v", err)
	}
	return command, token
}

func TestBeginExecutionFreezesBasis(t *testing.T) {
	now := testTime(2026, time.January, 1)
	command, token := approvedCommandWithToken(t)
	basis := validCalibration(now).Snapshot()

	if err := command.BeginExecution(token, "operator-chen", basis, testTime(2026, time.January, 3)); err != nil {
		t.Fatalf("begin execution: %v", err)
	}
	if command.Status != CommandExecuting || command.CalibrationBasis == nil {
		t.Fatalf("command did not enter executing with basis: %+v", command)
	}
	if command.CalibrationBasis.CalibrationID != "cal-1" {
		t.Fatalf("unexpected basis: %+v", command.CalibrationBasis)
	}
	if command.ExecutionToken != token {
		// token retained until terminal; basis should not clear it early
		t.Fatalf("token cleared during begin: %+v", command)
	}

	if err := command.Complete(testTime(2026, time.January, 3)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if command.CalibrationBasis == nil || command.CalibrationBasis.Reference != "BM-A" {
		t.Fatalf("completion removed the calibration basis")
	}
}

func TestBeginExecutionRejectsTerminalReplayBeforeBasis(t *testing.T) {
	now := testTime(2026, time.January, 1)
	command, token := approvedCommandWithToken(t)
	basis := validCalibration(now).Snapshot()
	if err := command.BeginExecution(token, "op", basis, testTime(2026, time.January, 3)); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	if err := command.Complete(testTime(2026, time.January, 3)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Replay fails on state, regardless of token validity or basis.
	err := command.BeginExecution(token, "op", CalibrationBasis{}, testTime(2026, time.January, 4))
	if err == nil || ErrorCodeOf(err) != "command_not_executable" {
		t.Fatalf("completed replay: got %v", err)
	}

	var domainErr *Error
	if errors.As(err, &domainErr) && domainErr.Kind != KindConflict {
		t.Fatalf("replay error kind: got %s", domainErr.Kind)
	}
}

func TestCurrentCalibrationPicksNewest(t *testing.T) {
	now := testTime(2026, time.January, 1)
	older := validCalibration(now)
	newer := validCalibration(now)
	newer.ID = "cal-2"

	if _, ok := CurrentCalibration(nil); ok {
		t.Fatal("expected no calibration for empty history")
	}
	current, ok := CurrentCalibration([]Calibration{older, newer})
	if !ok || current.ID != "cal-2" {
		t.Fatalf("expected newest calibration, got %+v ok=%v", current, ok)
	}
}
