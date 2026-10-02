package main

import (
	"fmt"
	"net/http"
	"time"
)

func fixtureGate(
	client *apiClient,
	id string,
	name string,
	maxAperture float64,
	initialAperture float64,
) (map[string]any, error) {
	result, err := client.post("/gates", map[string]any{
		"id":               id,
		"name":             name,
		"max_aperture":     maxAperture,
		"initial_aperture": initialAperture,
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusCreated, "create fixture gate"); err != nil {
		return nil, err
	}
	gate, err := objectField(result.Body, "gate")
	if err != nil {
		return nil, fmt.Errorf("create fixture gate response: %w", err)
	}
	return gate, nil
}

func fixtureCalibration(
	client *apiClient,
	gateID string,
	basis string,
	measuredAperture float64,
	validFor time.Duration,
	recordedBy string,
) (map[string]any, map[string]any, error) {
	now := time.Now().UTC()
	result, err := client.post("/gates/"+gateID+"/calibrations", map[string]any{
		"basis":             basis,
		"measured_aperture": measuredAperture,
		"valid_from":        now.Add(-time.Minute).Format(time.RFC3339),
		"valid_until":       now.Add(validFor).Format(time.RFC3339),
		"recorded_by":       recordedBy,
		"note":              "工作流检查登记",
	})
	if err != nil {
		return nil, nil, err
	}
	if err := requireStatus(result, http.StatusCreated, "register fixture calibration"); err != nil {
		return nil, nil, err
	}
	calibration, err := objectField(result.Body, "calibration")
	if err != nil {
		return nil, nil, fmt.Errorf("register fixture calibration response: %w", err)
	}
	return calibration, result.Body, nil
}

func fixtureCommand(
	client *apiClient,
	gateID string,
	targetAperture float64,
	reason string,
	requestedBy string,
) (map[string]any, error) {
	result, err := client.post("/commands", map[string]any{
		"gate_id":         gateID,
		"target_aperture": targetAperture,
		"reason":          reason,
		"requested_by":    requestedBy,
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusCreated, "create fixture command"); err != nil {
		return nil, err
	}
	command, err := objectField(result.Body, "command")
	if err != nil {
		return nil, fmt.Errorf("create fixture command response: %w", err)
	}
	return command, nil
}

func fixtureClearReview(
	client *apiClient,
	commandID string,
	verifier string,
) (map[string]any, error) {
	result, err := client.post("/commands/"+commandID+"/interlock", map[string]any{
		"verdict":  "clear",
		"verifier": verifier,
		"note":     "联锁条件满足",
	})
	if err != nil {
		return nil, err
	}
	if err := requireStatus(result, http.StatusOK, "clear fixture interlock"); err != nil {
		return nil, err
	}
	command, err := objectField(result.Body, "command")
	if err != nil {
		return nil, fmt.Errorf("clear fixture interlock response: %w", err)
	}
	return command, nil
}
