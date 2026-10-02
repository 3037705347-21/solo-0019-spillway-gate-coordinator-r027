package domain

import (
	"strings"
	"time"
)

type CalibrationStatus string

const (
	CalibrationActive     CalibrationStatus = "active"
	CalibrationSuperseded CalibrationStatus = "superseded"
)

// CalibrationBasis 是命令在执行瞬间所依赖的位置事实快照。
// 校准记录随后被取代或失效都不会改变命令上保留的依据。
type CalibrationBasis struct {
	CalibrationID    string
	Basis            string
	MeasuredAperture float64
}

type Calibration struct {
	ID               string
	GateID           string
	Basis            string
	MeasuredAperture float64
	ValidFrom        time.Time
	ValidUntil       time.Time
	RecordedBy       string
	Note             string
	Status           CalibrationStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Version          int64
}

func NewCalibration(
	calibrationID string,
	gateID string,
	basis string,
	measuredAperture float64,
	maxAperture float64,
	validFrom time.Time,
	validUntil time.Time,
	recordedBy string,
	note string,
	now time.Time,
) (Calibration, error) {
	calibrationID = strings.TrimSpace(calibrationID)
	if calibrationID == "" {
		return Calibration{}, Invalid("invalid_calibration_id", "calibration id is required")
	}

	gateID = strings.TrimSpace(gateID)
	if gateID == "" {
		return Calibration{}, Invalid("invalid_gate_reference", "gate id is required")
	}

	basis = strings.TrimSpace(basis)
	if basis == "" {
		return Calibration{}, Invalid("invalid_calibration_basis", "measurement basis is required")
	}

	recordedBy = strings.TrimSpace(recordedBy)
	if recordedBy == "" {
		return Calibration{}, Invalid("invalid_recorded_by", "recorded_by is required")
	}

	if measuredAperture < 0 || measuredAperture > maxAperture {
		return Calibration{}, Invalid("invalid_measured_aperture", "measured aperture must be within the gate limit")
	}

	validFrom = validFrom.UTC()
	validUntil = validUntil.UTC()
	now = now.UTC()
	if validFrom.IsZero() {
		return Calibration{}, Invalid("invalid_valid_from", "valid_from is required")
	}
	if validUntil.IsZero() {
		return Calibration{}, Invalid("invalid_valid_until", "valid_until is required")
	}
	if !validUntil.After(validFrom) {
		return Calibration{}, Invalid("invalid_validity_range", "valid_until must be after valid_from")
	}
	if validFrom.After(now) {
		return Calibration{}, Invalid("invalid_valid_from", "valid_from cannot be in the future")
	}
	if !validUntil.After(now) {
		return Calibration{}, Invalid("invalid_validity_range", "validity window must still be open at registration time")
	}

	return Calibration{
		ID:               calibrationID,
		GateID:           gateID,
		Basis:            basis,
		MeasuredAperture: measuredAperture,
		ValidFrom:        validFrom,
		ValidUntil:       validUntil,
		RecordedBy:       recordedBy,
		Note:             strings.TrimSpace(note),
		Status:           CalibrationActive,
		CreatedAt:        now,
		UpdatedAt:        now,
		Version:          1,
	}, nil
}

// ValidAt 判断校准在给定时刻是否处于有效窗口内（边界包含）。
func (c Calibration) ValidAt(at time.Time) bool {
	at = at.UTC()
	if at.Before(c.ValidFrom) {
		return false
	}
	return !at.After(c.ValidUntil)
}

// EnsureValidAt 在校准不是当前有效事实时返回稳定错误。
func (c Calibration) EnsureValidAt(at time.Time) error {
	at = at.UTC()
	if at.Before(c.ValidFrom) {
		return Conflict("calibration_not_active", "calibration is not active yet")
	}
	if at.After(c.ValidUntil) {
		return Conflict("calibration_expired", "calibration has passed its valid_until time")
	}
	return nil
}

func (c *Calibration) Supersede(now time.Time) error {
	if c.Status != CalibrationActive {
		return Conflict("calibration_not_active", "only the active calibration can be superseded")
	}
	c.Status = CalibrationSuperseded
	c.UpdatedAt = now.UTC()
	c.Version++
	return nil
}

func (c Calibration) BasisSnapshot() CalibrationBasis {
	return CalibrationBasis{
		CalibrationID:    c.ID,
		Basis:            c.Basis,
		MeasuredAperture: c.MeasuredAperture,
	}
}
