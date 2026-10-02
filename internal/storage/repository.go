package storage

import (
	"context"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

type Repository interface {
	CreateGate(ctx context.Context, gate domain.Gate) error
	Gate(ctx context.Context, id string) (domain.Gate, error)
	CreateCommand(ctx context.Context, command domain.GateCommand) error
	Command(ctx context.Context, id string) (domain.GateCommand, error)
	ReviewCommand(
		ctx context.Context,
		commandID string,
		decision domain.InterlockDecision,
		token string,
		now time.Time,
	) (domain.GateCommand, domain.Gate, *domain.Calibration, error)
	ExecuteCommand(
		ctx context.Context,
		commandID string,
		token string,
		operator string,
		simulateFault bool,
		now time.Time,
	) (domain.GateCommand, domain.Gate, *domain.Calibration, error)
	RegisterCalibration(
		ctx context.Context,
		gateID string,
		calibration domain.Calibration,
		now time.Time,
	) (domain.Calibration, domain.Gate, error)
	CalibrationHistory(ctx context.Context, gateID string) ([]domain.Calibration, error)
}
