package domain

import "time"

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
	ID               string                    `json:"id"`
	Code             string                    `json:"code"`
	FormulaID        string                    `json:"formula_id"`
	ProcessConfigKey string                    `json:"process_config_key"`
	VesselID         *string                   `json:"vessel_id,omitempty"`
	PlannedVolumeL   float64                   `json:"planned_volume_liters"`
	Notes            string                    `json:"notes,omitempty"`
	State            RunState                  `json:"state"`
	ProcessConfig    *ProcessConfigBinding     `json:"process_config,omitempty"`
	ProcessDecision  *ProcessSelectionDecision `json:"process_decision,omitempty"`
	StartedAt        *time.Time                `json:"started_at,omitempty"`
	EndedAt          *time.Time                `json:"ended_at,omitempty"`
	Observations     []GravityObservation      `json:"observations"`
	Metrics          *RunMetrics               `json:"metrics,omitempty"`
	Version          int64                     `json:"version"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
}

func (r FermentationRun) Clone() FermentationRun {
	cloned := r
	cloned.Observations = append([]GravityObservation(nil), r.Observations...)
	if r.VesselID != nil {
		value := *r.VesselID
		cloned.VesselID = &value
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
	if r.ProcessConfig != nil {
		value := *r.ProcessConfig
		cloned.ProcessConfig = &value
	}
	if r.ProcessDecision != nil {
		value := *r.ProcessDecision
		cloned.ProcessDecision = &value
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
	if r.ProcessConfigKey != "" {
		if err := ValidateIdentifier(r.ProcessConfigKey, "process_config_key"); err != nil {
			return err
		}
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

func (r *FermentationRun) StartFermentation(binding ProcessConfigBinding, decision ProcessSelectionDecision, now time.Time) error {
	if r.State != RunReserved || r.VesselID == nil {
		return NewError(CodeRunState, "only a reserved run can start fermentation")
	}
	stamp := now.UTC()
	binding.BoundAt = stamp
	r.State = RunFermenting
	r.ProcessConfig = &binding
	r.ProcessDecision = &decision
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
	if r.ProcessConfig == nil {
		return NewError(CodeProcessStandard, "run has no bound process configuration")
	}
	if err := ValidateObservation(*r.StartedAt, previous, observation, observation.ReceivedAt, *r.ProcessConfig); err != nil {
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
	if r.ProcessConfig == nil || r.StartedAt == nil {
		return NewError(CodeProcessStandard, "run has no bound process configuration")
	}
	if len(r.Observations) < r.ProcessConfig.ConditioningObservations {
		return NewError(CodeProcessStandard,
			"conditioning requires the number of observations bound at run start")
	}
	elapsed := now.UTC().Sub(r.StartedAt.UTC())
	if r.ProcessConfig.MinimumDays > 0 &&
		elapsed < time.Duration(r.ProcessConfig.MinimumDays)*24*time.Hour {
		return NewError(CodeProcessStandard,
			"conditioning was requested before the bound minimum fermentation duration")
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
	if r.ProcessConfig == nil || r.StartedAt == nil {
		return NewError(CodeProcessStandard, "run has no bound process configuration")
	}
	if len(r.Observations) < r.ProcessConfig.CompletionObservations {
		return NewError(CodeProcessStandard,
			"completion requires the number of observations bound at run start")
	}
	elapsed := now.UTC().Sub(r.StartedAt.UTC())
	if elapsed > time.Duration(r.ProcessConfig.MaximumDays)*24*time.Hour {
		return NewError(CodeProcessStandard,
			"completion was requested after the bound maximum fermentation duration")
	}
	stamp := now.UTC()
	final := r.Observations[len(r.Observations)-1]
	metrics := CalculateMetrics(originalGravity, final.Gravity, len(r.Observations))
	r.Metrics = &metrics
	r.State = RunCompleted
	r.EndedAt = &stamp
	r.UpdatedAt = stamp
	r.Version++
	return nil
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
