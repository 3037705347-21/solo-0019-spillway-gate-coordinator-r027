package coordination

import (
	"context"

	"example.com/spillway-gate-coordinator/internal/contracts"
	"example.com/spillway-gate-coordinator/internal/domain"
)

func (s *Service) RegisterGate(ctx context.Context, request contracts.RegisterGateRequest) (domain.Gate, error) {
	gate, err := domain.NewGate(
		request.ID,
		request.Name,
		request.MaxAperture,
		request.InitialAperture,
		s.now(),
	)
	if err != nil {
		return domain.Gate{}, err
	}
	if err := s.repository.CreateGate(ctx, gate); err != nil {
		return domain.Gate{}, err
	}
	return gate, nil
}

func (s *Service) Gate(ctx context.Context, id string) (domain.Gate, *domain.Calibration, error) {
	gate, err := s.repository.Gate(ctx, id)
	if err != nil {
		return domain.Gate{}, nil, err
	}
	if gate.CurrentCalibrationID == "" {
		return gate, nil, nil
	}
	history, err := s.repository.CalibrationHistory(ctx, id)
	if err != nil {
		return domain.Gate{}, nil, err
	}
	for i := range history {
		if history[i].ID == gate.CurrentCalibrationID {
			return gate, &history[i], nil
		}
	}
	return gate, nil, nil
}
