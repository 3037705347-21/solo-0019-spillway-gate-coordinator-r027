package coordination

import (
	"context"
	"time"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func (s *Service) RegisterCalibration(
	ctx context.Context,
	gateID string,
	request contracts.RegisterCalibrationRequest,
) (domain.Calibration, domain.Gate, error) {
	gate, err := s.repository.Gate(ctx, gateID)
	if err != nil {
		return domain.Calibration{}, domain.Gate{}, err
	}

	validFrom, err := parseCalibrationTime("valid_from", request.ValidFrom)
	if err != nil {
		return domain.Calibration{}, domain.Gate{}, err
	}
	validUntil, err := parseCalibrationTime("valid_until", request.ValidUntil)
	if err != nil {
		return domain.Calibration{}, domain.Gate{}, err
	}

	now := s.now()
	calibrationID, err := s.newID("cal")
	if err != nil {
		return domain.Calibration{}, domain.Gate{}, domain.Internal(
			"calibration_id_generation_failed",
			"calibration id could not be generated",
		)
	}

	calibration, err := domain.NewCalibration(
		calibrationID,
		gate.ID,
		request.Basis,
		request.MeasuredAperture,
		gate.MaxAperture,
		validFrom,
		validUntil,
		request.RecordedBy,
		request.Note,
		now,
	)
	if err != nil {
		return domain.Calibration{}, domain.Gate{}, err
	}

	return s.repository.RegisterCalibration(ctx, gate.ID, calibration, now)
}

func (s *Service) CalibrationHistory(ctx context.Context, gateID string) ([]domain.Calibration, error) {
	return s.repository.CalibrationHistory(ctx, gateID)
}

func parseCalibrationTime(field string, raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, domain.Invalid("invalid_"+field, field+" must be an RFC3339 timestamp")
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, domain.Invalid("invalid_"+field, field+" must be an RFC3339 timestamp")
	}
	return parsed, nil
}
