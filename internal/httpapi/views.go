package httpapi

import (
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func gateView(gate domain.Gate) contracts.GateView {
	return contracts.GateView{
		ID:              gate.ID,
		Name:            gate.Name,
		Status:          string(gate.Status),
		Aperture:        gate.Aperture,
		MaxAperture:     gate.MaxAperture,
		ActiveCommandID: gate.ActiveCommandID,
		UpdatedAt:       formatTime(gate.UpdatedAt),
		Version:         gate.Version,
	}
}

func calibrationView(calibration domain.Calibration) contracts.CalibrationView {
	return contracts.CalibrationView{
		ID:               calibration.ID,
		GateID:           calibration.GateID,
		Reference:        calibration.Reference,
		MeasuredAperture: calibration.MeasuredAperture,
		MinAperture:      calibration.MinAperture,
		MaxAperture:      calibration.MaxAperture,
		ValidFrom:        formatTime(calibration.ValidFrom),
		ValidUntil:       formatTime(calibration.ValidUntil),
		RegisteredBy:     calibration.RegisteredBy,
		Note:             calibration.Note,
		RegisteredAt:     formatTime(calibration.RegisteredAt),
	}
}

func calibrationBasisView(basis domain.CalibrationBasis) contracts.CalibrationView {
	return contracts.CalibrationView{
		ID:               basis.CalibrationID,
		Reference:        basis.Reference,
		MeasuredAperture: basis.MeasuredAperture,
		MinAperture:      basis.MinAperture,
		MaxAperture:      basis.MaxAperture,
		ValidFrom:        formatTime(basis.ValidFrom),
		ValidUntil:       formatTime(basis.ValidUntil),
		RegisteredBy:     basis.RegisteredBy,
		RegisteredAt:     formatTime(basis.RegisteredAt),
	}
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
	if command.CalibrationBasis != nil {
		basis := calibrationBasisView(*command.CalibrationBasis)
		view.CalibrationBasis = &basis
	}
	return view
}
