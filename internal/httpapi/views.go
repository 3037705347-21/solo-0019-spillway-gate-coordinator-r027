package httpapi

import (
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func calibrationView(calibration domain.Calibration, now time.Time) contracts.CalibrationView {
	return contracts.CalibrationView{
		ID:               calibration.ID,
		GateID:           calibration.GateID,
		Basis:            calibration.Basis,
		MeasuredAperture: calibration.MeasuredAperture,
		ValidFrom:        formatTime(calibration.ValidFrom),
		ValidUntil:       formatTime(calibration.ValidUntil),
		RecordedBy:       calibration.RecordedBy,
		Note:             calibration.Note,
		Status:           string(calibration.Status),
		CurrentlyValid:   calibration.Status == domain.CalibrationActive && calibration.ValidAt(now),
		CreatedAt:        formatTime(calibration.CreatedAt),
		UpdatedAt:        formatTime(calibration.UpdatedAt),
		Version:          calibration.Version,
	}
}

func gateView(gate domain.Gate, calibration *domain.Calibration, now time.Time) contracts.GateView {
	view := contracts.GateView{
		ID:                   gate.ID,
		Name:                 gate.Name,
		Status:               string(gate.Status),
		Aperture:             gate.Aperture,
		MaxAperture:          gate.MaxAperture,
		ActiveCommandID:      gate.ActiveCommandID,
		CurrentCalibrationID: gate.CurrentCalibrationID,
		UpdatedAt:            formatTime(gate.UpdatedAt),
		Version:              gate.Version,
	}
	if calibration != nil {
		embedded := calibrationView(*calibration, now)
		view.CurrentCalibration = &embedded
	}
	return view
}

func commandView(command domain.GateCommand, includeToken bool) contracts.CommandView {
	view := contracts.CommandView{
		ID:               command.ID,
		GateID:           command.GateID,
		TargetAperture:   command.TargetAperture,
		StartingAperture: command.StartingAperture,
		Reason:           command.Reason,
		RequestedBy:      command.RequestedBy,
		Status:           string(command.Status),
		ExecutedBy:       command.ExecutedBy,
		TerminalReason:   command.TerminalReason,
		CreatedAt:        formatTime(command.CreatedAt),
		UpdatedAt:        formatTime(command.UpdatedAt),
		Version:          command.Version,
	}

	if includeToken && command.Status == domain.CommandApproved {
		view.ExecutionToken = command.ExecutionToken
	}
	if command.Interlock != nil {
		view.Interlock = &contracts.InterlockDecisionView{
			Verdict:    string(command.Interlock.Verdict),
			Verifier:   command.Interlock.Verifier,
			Note:       command.Interlock.Note,
			ReviewedAt: formatTime(command.Interlock.ReviewedAt),
		}
	}
	if command.Calibration != nil {
		view.Calibration = &contracts.CalibrationBasisView{
			CalibrationID:    command.Calibration.CalibrationID,
			Basis:            command.Calibration.Basis,
			MeasuredAperture: command.Calibration.MeasuredAperture,
		}
	}
	return view
}
