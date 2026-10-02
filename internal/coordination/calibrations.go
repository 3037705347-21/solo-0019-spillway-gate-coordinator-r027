package coordination

import (
	"context"
	"strings"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func (s *Service) RegisterCalibration(
	ctx context.Context,
	gateID string,
	request contracts.RegisterCalibrationRequest,
) (domain.Calibration, error) {
	gate, err := s.repository.Gate(ctx, gateID)
	if err != nil {
		return domain.Calibration{}, err
	}

	validFrom, err := parseCalibrationTime(request.ValidFrom)
	if err != nil {
		return domain.Calibration{}, domain.Invalid("invalid_calibration_valid_from", "valid_from must be an RFC3339 timestamp")
	}
	validUntil, err := parseCalibrationTime(request.ValidUntil)
	if err != nil {
		return domain.Calibration{}, domain.Invalid("invalid_calibration_valid_until", "valid_until must be an RFC3339 timestamp")
	}

	calibrationID, err := s.newID("cal")
	if err != nil {
		return domain.Calibration{}, domain.Internal("calibration_id_generation_failed", "calibration id could not be generated")
	}

	calibration, err := domain.NewCalibration(
		calibrationID,
		gate.ID,
		request.Reference,
		request.MeasuredAperture,
		request.MinAperture,
		request.MaxAperture,
		gate.MaxAperture,
		validFrom,
		validUntil,
		request.RegisteredBy,
		request.Note,
		s.now(),
	)
	if err != nil {
		return domain.Calibration{}, err
	}

	return s.repository.RegisterCalibration(ctx, calibration)
}

func (s *Service) GateCalibrations(ctx context.Context, gateID string) ([]domain.Calibration, error) {
	return s.repository.GateCalibrations(ctx, gateID)
}

func parseCalibrationTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
