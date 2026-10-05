package workflowcheck

import (
	"fmt"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type formulaView struct {
	ID               string               `json:"id"`
	State            domain.FormulaState  `json:"state"`
	Version          int64                `json:"version"`
	ProcessConfigKey string               `json:"process_config_key"`
	Steps            []domain.FormulaStep `json:"steps"`
}

type processConfigView struct {
	ID                       string                    `json:"id"`
	Key                      string                    `json:"key"`
	ConfigVersion            int                       `json:"config_version"`
	Name                     string                    `json:"name"`
	State                    domain.ProcessConfigState `json:"state"`
	Version                  int64                     `json:"version"`
	MinFermentationTempC     float64                   `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64                   `json:"max_fermentation_temp_c"`
	MinimumDays              int                       `json:"minimum_days"`
	MaximumDays              int                       `json:"maximum_days"`
	ConditioningObservations int                       `json:"conditioning_observations"`
	CompletionObservations   int                       `json:"completion_observations"`
	EffectiveAt              *time.Time                `json:"effective_at"`
}

type processBindingView struct {
	ID                       string    `json:"id"`
	Key                      string    `json:"key"`
	ConfigVersion            int       `json:"config_version"`
	Name                     string    `json:"name"`
	MinFermentationTempC     float64   `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64   `json:"max_fermentation_temp_c"`
	ConditioningObservations int       `json:"conditioning_observations"`
	CompletionObservations   int       `json:"completion_observations"`
	EffectiveAt              time.Time `json:"effective_at"`
	BoundAt                  time.Time `json:"bound_at"`
}

type processDecisionView struct {
	Key                      string                     `json:"key"`
	EvaluatedAt              time.Time                  `json:"evaluated_at"`
	CandidatesConsidered     int                        `json:"candidates_considered"`
	SelectedConfigID         string                     `json:"selected_config_id"`
	SelectedConfigVersion    int                        `json:"selected_config_version"`
	Reason                   string                     `json:"reason"`
	Basis                    string                     `json:"basis"`
	ConsideredConfigurations []processConsiderationView `json:"considered_configurations"`
}

type processConsiderationView struct {
	ID            string     `json:"id"`
	ConfigVersion int        `json:"config_version"`
	State         string     `json:"state"`
	EffectiveAt   *time.Time `json:"effective_at"`
	PublishedAt   *time.Time `json:"published_at"`
	WithdrawnAt   *time.Time `json:"withdrawn_at"`
	Eligible      bool       `json:"eligible"`
	Reason        string     `json:"reason"`
}

type vesselView struct {
	ID          string             `json:"id"`
	State       domain.VesselState `json:"state"`
	Version     int64              `json:"version"`
	ActiveRunID *string            `json:"active_run_id"`
}

type runView struct {
	ID              string                      `json:"id"`
	State           domain.RunState             `json:"state"`
	Version         int64                       `json:"version"`
	VesselID        *string                     `json:"vessel_id"`
	ProcessConfig   *processBindingView         `json:"process_config"`
	ProcessDecision *processDecisionView        `json:"process_decision"`
	Observations    []domain.GravityObservation `json:"observations"`
	Metrics         *domain.RunMetrics          `json:"metrics"`
}

type pairView struct {
	Run    runView    `json:"run"`
	Vessel vesselView `json:"vessel"`
}

func createFormulaPayload(name string) map[string]any {
	return createFormulaPayloadWithKey(name, "")
}

func createFormulaPayloadWithKey(name, key string) map[string]any {
	payload := map[string]any{
		"name":                    name,
		"style":                   "Cellar pilot",
		"target_original_gravity": 1.052,
		"target_final_gravity":    1.012,
		"max_fermentation_temp_c": 22.0,
		"minimum_days":            7,
		"maximum_days":            21,
	}
	if key != "" {
		payload["process_config_key"] = key
	}
	return payload
}

func createProcessConfigPayload(key, name string, minTemp, maxTemp float64) map[string]any {
	return map[string]any{
		"key":                       key,
		"name":                      name,
		"min_fermentation_temp_c":   minTemp,
		"max_fermentation_temp_c":   maxTemp,
		"minimum_days":              0,
		"maximum_days":              120,
		"conditioning_observations": 2,
		"completion_observations":   3,
	}
}

func formulaStepPayload() map[string]any {
	return map[string]any{
		"steps": []map[string]any{
			{
				"sequence":      1,
				"label":         "Heat and combine",
				"temperature_c": 66.0,
				"hold_minutes":  60,
			},
			{
				"sequence":      2,
				"label":         "Transfer and cool",
				"temperature_c": 20.0,
				"hold_minutes":  30,
			},
			{
				"sequence":      3,
				"label":         "Hold for primary activity",
				"temperature_c": 19.0,
				"hold_minutes":  240,
			},
		},
	}
}

func createRunPayload(code, formulaID string) map[string]any {
	return map[string]any{
		"code":                  code,
		"formula_id":            formulaID,
		"planned_volume_liters": 180.0,
		"notes":                 "pilot run",
	}
}

func verifiedAt(base time.Time, sequence int) time.Time {
	return base.Add(time.Duration(sequence) * 10 * time.Second)
}

func requireRunState(run runView, state domain.RunState) error {
	if run.State != state {
		return fmt.Errorf("expected run state %q, got %q", state, run.State)
	}
	return nil
}

func requireVesselState(vessel vesselView, state domain.VesselState) error {
	if vessel.State != state {
		return fmt.Errorf("expected vessel state %q, got %q", state, vessel.State)
	}
	return nil
}
