package domain

import (
	"strings"
	"time"
)

// Calibration is an independently registered position fact for a gate. It
// records the measurement reference it was taken against, the aperture value
// measured at that reference, the aperture interval the measurement can be
// trusted for, and the time window in which the fact is current.
type Calibration struct {
	ID               string
	GateID           string
	Reference        string
	MeasuredAperture float64
	MinAperture      float64
	MaxAperture      float64
	ValidFrom        time.Time
	ValidUntil       time.Time
	RegisteredBy     string
	Note             string
	RegisteredAt     time.Time
	Version          int64
}

// CalibrationBasis is the immutable calibration evidence captured by a command
// at the moment execution begins. It survives later recalibration so completed
// and aborted commands always show the fact they actually moved against.
type CalibrationBasis struct {
	CalibrationID    string
	Reference        string
	MeasuredAperture float64
	MinAperture      float64
	MaxAperture      float64
	ValidFrom        time.Time
	ValidUntil       time.Time
	RegisteredBy     string
	RegisteredAt     time.Time
}

func NewCalibration(
	calibrationID string,
	gateID string,
	reference string,
	measuredAperture float64,
	minAperture float64,
	maxAperture float64,
	gateMaxAperture float64,
	validFrom time.Time,
	validUntil time.Time,
	registeredBy string,
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

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Calibration{}, Invalid("invalid_calibration_reference", "measurement reference is required")
	}

	registeredBy = strings.TrimSpace(registeredBy)
	if registeredBy == "" {
		return Calibration{}, Invalid("invalid_calibration_registered_by", "registered_by is required")
	}

	if maxAperture <= 0 {
		return Calibration{}, Invalid("invalid_calibration_range", "calibration max aperture must be greater than zero")
	}
	if maxAperture > gateMaxAperture {
		return Calibration{}, Invalid("invalid_calibration_range", "calibration range must stay within the gate max aperture")
	}
	if minAperture < 0 || minAperture > maxAperture {
		return Calibration{}, Invalid("invalid_calibration_range", "calibration min aperture must be within zero and the max aperture")
	}
	if measuredAperture < minAperture || measuredAperture > maxAperture {
		return Calibration{}, Invalid("invalid_measured_aperture", "measured aperture must fall within the calibrated aperture range")
	}

	validFrom = validFrom.UTC()
	validUntil = validUntil.UTC()
	if !validUntil.After(validFrom) {
		return Calibration{}, Invalid("invalid_calibration_validity", "valid_until must be after valid_from")
	}

	return Calibration{
		ID:               calibrationID,
		GateID:           gateID,
		Reference:        reference,
		MeasuredAperture: measuredAperture,
		MinAperture:      minAperture,
		MaxAperture:      maxAperture,
		ValidFrom:        validFrom,
		ValidUntil:       validUntil,
		RegisteredBy:     registeredBy,
		Note:             strings.TrimSpace(note),
		RegisteredAt:     now.UTC(),
		Version:          1,
	}, nil
}

// IsValidAt reports whether the calibration is in effect at moment. The
// validity window is half-open: ValidFrom <= moment < ValidUntil, so a command
// started exactly at expiry is rejected consistently everywhere.
func (c Calibration) IsValidAt(moment time.Time) bool {
	moment = moment.UTC()
	return !moment.Before(c.ValidFrom) && moment.Before(c.ValidUntil)
}

// Covers reports whether aperture lies inside the trusted measurement interval.
func (c Calibration) Covers(aperture float64) bool {
	return aperture >= c.MinAperture && aperture <= c.MaxAperture
}

// GuardExecution accepts a movement only while the calibration is current and
// both endpoints of the movement stay inside its trusted aperture interval.
func (c Calibration) GuardExecution(startingAperture, targetAperture float64, moment time.Time) error {
	if !c.IsValidAt(moment) {
		return Conflict("calibration_not_current", "gate calibration is missing, not yet in effect, or expired")
	}
	if !c.Covers(startingAperture) || !c.Covers(targetAperture) {
		return Conflict("aperture_outside_calibration", "the movement leaves the trusted aperture range of the current calibration")
	}
	return nil
}

// CurrentCalibration returns the authoritative calibration for a gate. The
// most recently registered fact supersedes every earlier fact: callers must
// not fall back to an older fact while the newest one is pending or expired,
// otherwise overlapping trust windows would make movements ambiguous.
//
// It returns false only when no fact has ever been registered; the caller
// reports that as an unknown baseline, distinct from a stale newest fact.
func CurrentCalibration(calibrations []Calibration) (Calibration, bool) {
	if len(calibrations) == 0 {
		return Calibration{}, false
	}
	return calibrations[len(calibrations)-1], true
}

func (c Calibration) Snapshot() CalibrationBasis {
	return CalibrationBasis{
		CalibrationID:    c.ID,
		Reference:        c.Reference,
		MeasuredAperture: c.MeasuredAperture,
		MinAperture:      c.MinAperture,
		MaxAperture:      c.MaxAperture,
		ValidFrom:        c.ValidFrom,
		ValidUntil:       c.ValidUntil,
		RegisteredBy:     c.RegisteredBy,
		RegisteredAt:     c.RegisteredAt,
	}
}
