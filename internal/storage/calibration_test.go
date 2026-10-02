package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

var (
	calibWindowStart = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	calibWindowEnd   = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
)

func setupCalibratedGate(t *testing.T, repo *MemoryRepository, gateID string, max float64) (domain.Calibration, domain.GateCommand, string) {
	t.Helper()
	ctx := context.Background()
	gate, err := domain.NewGate(gateID, "校准闸门", max, 0.5, time.Now())
	if err != nil {
		t.Fatalf("new gate: %v", err)
	}
	if err := repo.CreateGate(ctx, gate); err != nil {
		t.Fatalf("create gate: %v", err)
	}
	calibration, err := domain.NewCalibration(
		"cal-"+gateID, gateID, "BM-A", 0.5, 0, max, max,
		calibWindowStart, calibWindowEnd, "maintainer", "", time.Now(),
	)
	if err != nil {
		t.Fatalf("new calibration: %v", err)
	}
	if _, err := repo.RegisterCalibration(ctx, calibration); err != nil {
		t.Fatalf("register calibration: %v", err)
	}
	command, err := domain.NewGateCommand(
		"cmd-"+gateID, gateID, 2.0, 0.5, max, "reason", "dispatcher", time.Now(),
	)
	if err != nil {
		t.Fatalf("new command: %v", err)
	}
	if err := repo.CreateCommand(ctx, command); err != nil {
		t.Fatalf("create command: %v", err)
	}
	decision, err := domain.NewInterlockDecision("clear", "safety", "", time.Now())
	if err != nil {
		t.Fatalf("new decision: %v", err)
	}
	approved, _, err := repo.ReviewCommand(ctx, command.ID, decision, "token-"+gateID, time.Now())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	return calibration, approved, "token-" + gateID
}

func TestExecuteWithoutCalibrationIsRejected(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	gate, err := domain.NewGate("gate-bare", "无基准闸门", 4, 0.5, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateGate(ctx, gate); err != nil {
		t.Fatal(err)
	}
	command, err := domain.NewGateCommand("cmd-bare", "gate-bare", 2, 0.5, 4, "r", "d", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateCommand(ctx, command); err != nil {
		t.Fatal(err)
	}
	decision, _ := domain.NewInterlockDecision("clear", "safety", "", time.Now())
	if _, _, err := repo.ReviewCommand(ctx, command.ID, decision, "tok", time.Now()); err != nil {
		t.Fatal(err)
	}

	_, _, err = repo.ExecuteCommand(ctx, command.ID, "tok", "op", false, time.Now())
	if err == nil || domain.ErrorCodeOf(err) != "calibration_unknown" {
		t.Fatalf("expected calibration_unknown, got %v", err)
	}
}

func TestTerminalCommandKeepsBasisAfterRecalibration(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	calibration, command, token := setupCalibratedGate(t, repo, "gate-term", 4)

	completed, _, err := repo.ExecuteCommand(ctx, command.ID, token, "op", false, time.Now())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if completed.CalibrationBasis == nil || completed.CalibrationBasis.CalibrationID != calibration.ID {
		t.Fatalf("completed command missing original basis: %+v", completed.CalibrationBasis)
	}

	replacement, err := domain.NewCalibration(
		"cal-new", "gate-term", "BM-B", 2, 0, 4, 4,
		calibWindowStart, calibWindowEnd, "maintainer", "", time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterCalibration(ctx, replacement); err != nil {
		t.Fatal(err)
	}

	reread, err := repo.Command(ctx, command.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.CalibrationBasis == nil || reread.CalibrationBasis.CalibrationID != calibration.ID {
		t.Fatalf("recalibration rewrote historical basis: %+v", reread.CalibrationBasis)
	}

	_, _, err = repo.ExecuteCommand(ctx, command.ID, token, "op", false, time.Now())
	if err == nil || domain.ErrorCodeOf(err) != "command_not_executable" {
		t.Fatalf("terminal replay: got %v", err)
	}
}

// TestExecuteAndRecalibrateConcurrent interleaves execution with an
// immediately-expiring recalibration under the repository lock. The two
// operations must serialize to exactly one consistent outcome: either the
// movement used the old fact and the command is completed, or the expired
// fact won and the approved command is untouched with the gate reserved.
func TestExecuteAndRecalibrateConcurrent(t *testing.T) {
	const iterations = 100
	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		repo := NewMemoryRepository()
		gateID := "gate-conc"
		oldCalibration, command, token := setupCalibratedGate(t, repo, gateID, 4)

		expired, err := domain.NewCalibration(
			"cal-expired", gateID, "BM-EXPIRED", 0.5, 0, 4, 4,
			time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC),
			"maintainer", "", time.Now(),
		)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var executeErr error
		var registerErr error

		go func() {
			defer wg.Done()
			<-start
			_, _, executeErr = repo.ExecuteCommand(ctx, command.ID, token, "op", false, time.Now())
		}()
		go func() {
			defer wg.Done()
			<-start
			_, registerErr = repo.RegisterCalibration(ctx, expired)
		}()
		close(start)
		wg.Wait()

		if registerErr != nil {
			t.Fatalf("recalibration must never be blocked: %v", registerErr)
		}

		finalCommand, err := repo.Command(ctx, command.ID)
		if err != nil {
			t.Fatal(err)
		}
		finalGate, err := repo.Gate(ctx, gateID)
		if err != nil {
			t.Fatal(err)
		}

		switch finalCommand.Status {
		case domain.CommandCompleted:
			if executeErr != nil {
				t.Fatalf("completed command returned error: %v", executeErr)
			}
			if finalGate.Status != domain.GateIdle || finalGate.ActiveCommandID != "" {
				t.Fatalf("completed left gate inconsistent: %+v", finalGate)
			}
			if finalGate.Aperture != 2.0 {
				t.Fatalf("completed gate aperture: got %v", finalGate.Aperture)
			}
			if finalCommand.CalibrationBasis == nil ||
				finalCommand.CalibrationBasis.CalibrationID != oldCalibration.ID {
				t.Fatalf("winning execution must cite the old fact: %+v", finalCommand.CalibrationBasis)
			}
		case domain.CommandApproved:
			if executeErr == nil || domain.ErrorCodeOf(executeErr) != "calibration_not_current" {
				t.Fatalf("lost execution expected calibration_not_current, got: %v", executeErr)
			}
			if finalGate.Status != domain.GateReserved || finalGate.ActiveCommandID != command.ID {
				t.Fatalf("rejected execution changed gate state: %+v", finalGate)
			}
			if finalCommand.CalibrationBasis != nil {
				t.Fatalf("rejected execution wrote a calibration basis")
			}
		default:
			t.Fatalf("unexpected command status after concurrent operations: %s", finalCommand.Status)
		}
	}
}

func TestRecalibrationCannotReleaseReservation(t *testing.T) {
	repo := NewMemoryRepository()
	ctx := context.Background()
	_, command, _ := setupCalibratedGate(t, repo, "gate-reserve", 4)

	gateBefore, _ := repo.Gate(ctx, "gate-reserve")
	if gateBefore.Status != domain.GateReserved {
		t.Fatalf("precondition: gate reserved, got %s", gateBefore.Status)
	}

	replacement, err := domain.NewCalibration(
		"cal-replacement", "gate-reserve", "BM-B", 0.5, 0, 4, 4,
		calibWindowStart, calibWindowEnd, "maintainer", "", time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterCalibration(ctx, replacement); err != nil {
		t.Fatal(err)
	}

	gateAfter, _ := repo.Gate(ctx, "gate-reserve")
	if gateAfter.Status != domain.GateReserved || gateAfter.ActiveCommandID != command.ID {
		t.Fatalf("recalibration altered the reservation: %+v", gateAfter)
	}

	history, err := repo.GateCalibrations(ctx, "gate-reserve")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected two calibration facts, got %d", len(history))
	}
	current, ok := domain.CurrentCalibration(history)
	if !ok || current.ID != "cal-replacement" {
		t.Fatalf("newest fact not authoritative: %+v", current)
	}
}
