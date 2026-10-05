package domain

import (
	"fmt"
	"time"
)

type RunState string

const (
	RunPlanned      RunState = "planned"
	RunReserved     RunState = "reserved"
	RunFermenting   RunState = "fermenting"
	RunConditioning RunState = "conditioning"
	RunCompleted    RunState = "completed"
	RunAborted      RunState = "aborted"
)

type FermentationRun struct {
	ID             string               `json:"id"`
	Code           string               `json:"code"`
	FormulaID      string               `json:"formula_id"`
	VesselID       *string              `json:"vessel_id,omitempty"`
	PlannedVolumeL float64              `json:"planned_volume_liters"`
	Notes          string               `json:"notes,omitempty"`
	State          RunState             `json:"state"`
	ProfileBinding *ProfileBinding      `json:"profile_binding,omitempty"`
	StartedAt      *time.Time           `json:"started_at,omitempty"`
	EndedAt        *time.Time           `json:"ended_at,omitempty"`
	Observations   []GravityObservation `json:"observations"`
	Metrics        *RunMetrics          `json:"metrics,omitempty"`
	Version        int64                `json:"version"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

func (r FermentationRun) Clone() FermentationRun {
	cloned := r
	cloned.Observations = append([]GravityObservation(nil), r.Observations...)
	if r.VesselID != nil {
		value := *r.VesselID
		cloned.VesselID = &value
	}
	if r.ProfileBinding != nil {
		value := *r.ProfileBinding
		cloned.ProfileBinding = &value
	}
	if r.StartedAt != nil {
		value := *r.StartedAt
		cloned.StartedAt = &value
	}
	if r.EndedAt != nil {
		value := *r.EndedAt
		cloned.EndedAt = &value
	}
	if r.Metrics != nil {
		value := *r.Metrics
		cloned.Metrics = &value
	}
	return cloned
}

func (r FermentationRun) Validate() error {
	if err := ValidateRunCode(r.Code, "code"); err != nil {
		return err
	}
	if r.PlannedVolumeL <= 0 {
		return FieldError(CodeInvalidInput, "planned_volume_liters",
			"must be greater than zero")
	}
	if len([]rune(r.Notes)) > 1000 {
		return FieldError(CodeInvalidInput, "notes",
			"must not exceed 1000 characters")
	}
	return nil
}

func (r *FermentationRun) ReserveVessel(vesselID string, now time.Time) error {
	if r.State != RunPlanned {
		return NewError(CodeRunState, "only a planned run can reserve a vessel")
	}
	value := vesselID
	r.VesselID = &value
	r.State = RunReserved
	r.UpdatedAt = now.UTC()
	r.Version++
	return nil
}

func (r *FermentationRun) StartFermentation(now time.Time, binding ProfileBinding) error {
	if r.State != RunReserved || r.VesselID == nil {
		return NewError(CodeRunState, "only a reserved run can start fermentation")
	}
	stamp := now.UTC()
	binding.BoundAt = stamp
	r.ProfileBinding = &binding
	r.State = RunFermenting
	r.StartedAt = &stamp
	r.UpdatedAt = stamp
	r.Version++
	return nil
}

func (r *FermentationRun) AddObservation(observation GravityObservation) error {
	if r.State != RunFermenting || r.StartedAt == nil {
		return NewError(CodeRunState, "observations require a fermenting run")
	}
	if len(r.Observations) >= 500 {
		return NewError(CodeObservationConflict,
			"run cannot contain more than 500 observations")
	}
	var previous *GravityObservation
	if len(r.Observations) > 0 {
		value := r.Observations[len(r.Observations)-1]
		previous = &value
	}
	if err := ValidateObservation(*r.StartedAt, previous, observation, observation.ReceivedAt); err != nil {
		return err
	}
	expected := len(r.Observations) + 1
	if observation.Sequence != expected {
		return NewError(CodeObservationConflict,
			"observation sequence must be contiguous and begin at 1")
	}
	observation.RunID = r.ID
	r.Observations = append(r.Observations, observation)
	r.UpdatedAt = observation.ReceivedAt.UTC()
	r.Version++
	return nil
}

func (r *FermentationRun) BeginConditioning(now time.Time) error {
	if r.State != RunFermenting {
		return NewError(CodeRunState, "only a fermenting run can enter conditioning")
	}
	required := r.profileParameters().MinObservationsToCondition
	if len(r.Observations) < required {
		return NewError(CodeRunState, fmt.Sprintf(
			"conditioning requires at least %d observations under %s",
			required, r.profileDescription()))
	}
	r.State = RunConditioning
	r.UpdatedAt = now.UTC()
	r.Version++
	return nil
}

func (r *FermentationRun) Complete(originalGravity float64, now time.Time) error {
	if r.State != RunConditioning {
		return NewError(CodeRunState, "only a conditioning run can be completed")
	}
	params := r.profileParameters()
	if len(r.Observations) < params.MinObservationsToComplete {
		return NewError(CodeRunState, fmt.Sprintf(
			"completion requires at least %d observations under %s",
			params.MinObservationsToComplete, r.profileDescription()))
	}
	stamp := now.UTC()
	if r.StartedAt != nil {
		elapsed := stamp.Sub(r.StartedAt.UTC())
		if minimum := time.Duration(params.MinFermentDays) * 24 * time.Hour; elapsed < minimum {
			return NewError(CodeProfileViolation, fmt.Sprintf(
				"%s expects at least %d days of fermentation",
				r.profileDescription(), params.MinFermentDays))
		}
		if maximum := time.Duration(params.MaxFermentDays) * 24 * time.Hour; elapsed > maximum {
			return NewError(CodeProfileViolation, fmt.Sprintf(
				"%s expects at most %d days of fermentation",
				r.profileDescription(), params.MaxFermentDays))
		}
	}
	final := r.Observations[len(r.Observations)-1]
	if final.TemperatureC < params.MinFermentationTempC ||
		final.TemperatureC > params.MaxFermentationTempC {
		return NewError(CodeProfileViolation, fmt.Sprintf(
			"%s expects a fermentation temperature between %.1f and %.1f Celsius",
			r.profileDescription(), params.MinFermentationTempC, params.MaxFermentationTempC))
	}
	metrics := CalculateMetrics(originalGravity, final.Gravity, len(r.Observations))
	r.Metrics = &metrics
	r.State = RunCompleted
	r.EndedAt = &stamp
	r.UpdatedAt = stamp
	r.Version++
	return nil
}

func (r FermentationRun) profileParameters() ProfileParameters {
	if r.ProfileBinding == nil {
		return DefaultProfileParameters()
	}
	return r.ProfileBinding.Parameters
}

func (r FermentationRun) profileDescription() string {
	if r.ProfileBinding == nil {
		return ProfileBinding{}.Describe()
	}
	return r.ProfileBinding.Describe()
}

func (r *FermentationRun) Abort(now time.Time) error {
	if r.State == RunCompleted || r.State == RunAborted {
		return NewError(CodeRunState, "completed or aborted runs cannot be aborted")
	}
	stamp := now.UTC()
	r.State = RunAborted
	r.EndedAt = &stamp
	r.UpdatedAt = stamp
	r.Version++
	return nil
}

func (r FermentationRun) HadStarted() bool {
	return r.StartedAt != nil
}
