package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"example.com/spillway-gate-coordinator/internal/domain"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	gates        map[string]domain.Gate
	commands     map[string]domain.GateCommand
	calibrations map[string][]domain.Calibration
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		gates:        make(map[string]domain.Gate),
		commands:     make(map[string]domain.GateCommand),
		calibrations: make(map[string][]domain.Calibration),
	}
}

func (m *MemoryRepository) CreateGate(_ context.Context, gate domain.Gate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.gates[gate.ID]; exists {
		return domain.Conflict("duplicate_gate_id", "gate id already exists")
	}
	m.gates[gate.ID] = gate
	return nil
}

func (m *MemoryRepository) Gate(_ context.Context, id string) (domain.Gate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gate, exists := m.gates[id]
	if !exists {
		return domain.Gate{}, domain.NotFound("gate_not_found", "gate id was not found")
	}
	return gate, nil
}

func (m *MemoryRepository) CreateCommand(_ context.Context, command domain.GateCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.NotFound("gate_not_found", "gate id was not found")
	}
	if gate.Status == domain.GateFaulted {
		return domain.Conflict("gate_faulted", "faulted gates cannot receive new commands")
	}
	if command.StartingAperture != gate.Aperture {
		return domain.Conflict("gate_aperture_changed", "gate aperture changed before the command was stored")
	}
	if command.TargetAperture > gate.MaxAperture {
		return domain.Invalid("invalid_target_aperture", "target aperture is outside the gate limit")
	}
	if _, exists := m.commands[command.ID]; exists {
		return domain.Conflict("duplicate_command_id", "command id already exists")
	}

	m.commands[command.ID] = command
	return nil
}

func (m *MemoryRepository) Command(_ context.Context, id string) (domain.GateCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	command, exists := m.commands[id]
	if !exists {
		return domain.GateCommand{}, domain.NotFound("command_not_found", "command id was not found")
	}
	return cloneCommand(command), nil
}

func (m *MemoryRepository) ReviewCommand(
	_ context.Context,
	commandID string,
	decision domain.InterlockDecision,
	token string,
	now time.Time,
) (domain.GateCommand, domain.Gate, *domain.Calibration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	command, exists := m.commands[commandID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, nil, domain.NotFound("command_not_found", "command id was not found")
	}
	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, nil, domain.NotFound("gate_not_found", "gate id was not found")
	}

	nextCommand := cloneCommand(command)
	if err := nextCommand.Review(decision, token, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, nil, err
	}

	nextGate := gate
	if decision.IsClear() {
		if err := nextGate.Reserve(nextCommand.ID, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, nil, err
		}
	}

	m.commands[nextCommand.ID] = nextCommand
	m.gates[nextGate.ID] = nextGate
	return cloneCommand(nextCommand), nextGate, m.activeCalibrationLocked(nextGate), nil
}

func (m *MemoryRepository) ExecuteCommand(
	_ context.Context,
	commandID string,
	token string,
	operator string,
	simulateFault bool,
	now time.Time,
) (domain.GateCommand, domain.Gate, *domain.Calibration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	command, exists := m.commands[commandID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, nil, domain.NotFound("command_not_found", "command id was not found")
	}
	gate, exists := m.gates[command.GateID]
	if !exists {
		return domain.GateCommand{}, domain.Gate{}, nil, domain.NotFound("gate_not_found", "gate id was not found")
	}

	nextCommand := cloneCommand(command)
	if err := nextCommand.BeginExecution(token, operator, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, nil, err
	}

	nextGate := gate
	if err := nextGate.StartMovement(nextCommand.ID, now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, nil, err
	}

	// 状态、令牌和占用通过后，才校验闸门当前校准事实：未知基准与校准
	// 失效都在此拒绝。上面的修改只存在于临界区内的副本上，拒绝即不写回，
	// 命令保持 approved、闸门保持 reserved。
	calibration := m.activeCalibrationLocked(nextGate)
	if calibration == nil {
		return domain.GateCommand{}, domain.Gate{}, nil, domain.Conflict(
			"calibration_unknown",
			"gate has no registered position calibration",
		)
	}
	if err := calibration.EnsureValidAt(now); err != nil {
		return domain.GateCommand{}, domain.Gate{}, nil, err
	}
	if err := nextCommand.BindCalibrationBasis(calibration.BasisSnapshot()); err != nil {
		return domain.GateCommand{}, domain.Gate{}, nil, err
	}

	if simulateFault {
		if err := nextGate.Settle(nextCommand.TargetAperture, true, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, nil, err
		}
		if err := nextCommand.Abort("simulated_gate_fault", now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, nil, err
		}
	} else {
		if err := nextGate.Settle(nextCommand.TargetAperture, false, now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, nil, err
		}
		if err := nextCommand.Complete(now); err != nil {
			return domain.GateCommand{}, domain.Gate{}, nil, err
		}
	}

	m.commands[nextCommand.ID] = nextCommand
	m.gates[nextGate.ID] = nextGate
	return cloneCommand(nextCommand), nextGate, calibration, nil
}

func (m *MemoryRepository) RegisterCalibration(
	_ context.Context,
	gateID string,
	calibration domain.Calibration,
	now time.Time,
) (domain.Calibration, domain.Gate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	gate, exists := m.gates[gateID]
	if !exists {
		return domain.Calibration{}, domain.Gate{}, domain.NotFound("gate_not_found", "gate id was not found")
	}
	if calibration.GateID != gate.ID {
		return domain.Calibration{}, domain.Gate{}, domain.Invalid("invalid_gate_reference", "calibration does not reference the target gate")
	}
	if calibration.MeasuredAperture > gate.MaxAperture {
		return domain.Calibration{}, domain.Gate{}, domain.Invalid("invalid_measured_aperture", "measured aperture is outside the gate limit")
	}

	for _, existing := range m.calibrations[gateID] {
		if existing.ID == calibration.ID {
			return domain.Calibration{}, domain.Gate{}, domain.Conflict("duplicate_calibration_id", "calibration id already exists")
		}
	}

	nextGate := gate
	if err := nextGate.Recalibrate(calibration.ID, calibration.MeasuredAperture, now); err != nil {
		return domain.Calibration{}, domain.Gate{}, err
	}

	history := m.calibrations[gateID]
	for i := range history {
		if history[i].Status == domain.CalibrationActive {
			if err := history[i].Supersede(now); err != nil {
				return domain.Calibration{}, domain.Gate{}, err
			}
		}
	}
	history = append(history, calibration)
	m.calibrations[gateID] = history
	m.gates[gateID] = nextGate
	return calibration, nextGate, nil
}

func (m *MemoryRepository) CalibrationHistory(_ context.Context, gateID string) ([]domain.Calibration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.gates[gateID]; !exists {
		return nil, domain.NotFound("gate_not_found", "gate id was not found")
	}

	history := append([]domain.Calibration(nil), m.calibrations[gateID]...)
	sort.Slice(history, func(i, j int) bool {
		if history[i].CreatedAt.Equal(history[j].CreatedAt) {
			return history[i].ID > history[j].ID
		}
		return history[i].CreatedAt.After(history[j].CreatedAt)
	})
	return history, nil
}

// activeCalibrationLocked 调用方必须持有 m.mu。
func (m *MemoryRepository) activeCalibrationLocked(gate domain.Gate) *domain.Calibration {
	if gate.CurrentCalibrationID == "" {
		return nil
	}
	for _, calibration := range m.calibrations[gate.ID] {
		if calibration.ID == gate.CurrentCalibrationID {
			calibration := calibration
			return &calibration
		}
	}
	return nil
}

func cloneCommand(command domain.GateCommand) domain.GateCommand {
	if command.Interlock != nil {
		decision := *command.Interlock
		command.Interlock = &decision
	}
	if command.Calibration != nil {
		basis := *command.Calibration
		command.Calibration = &basis
	}
	return command
}
