package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ProcessConfigState string

const (
	ProcessConfigDraft     ProcessConfigState = "draft"
	ProcessConfigPublished ProcessConfigState = "published"
	ProcessConfigWithdrawn ProcessConfigState = "withdrawn"
)

const ProcessSelectionBasis = "highest effective_at not after evaluated_at; ties resolved by config_version, published_at, and id"

type ProcessConfig struct {
	ID                       string             `json:"id"`
	Key                      string             `json:"key"`
	ConfigVersion            int                `json:"config_version"`
	Name                     string             `json:"name"`
	MinFermentationTempC     float64            `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64            `json:"max_fermentation_temp_c"`
	MinimumDays              int                `json:"minimum_days"`
	MaximumDays              int                `json:"maximum_days"`
	ConditioningObservations int                `json:"conditioning_observations"`
	CompletionObservations   int                `json:"completion_observations"`
	State                    ProcessConfigState `json:"state"`
	EffectiveAt              *time.Time         `json:"effective_at,omitempty"`
	PublishedAt              *time.Time         `json:"published_at,omitempty"`
	WithdrawnAt              *time.Time         `json:"withdrawn_at,omitempty"`
	Version                  int64              `json:"version"`
	CreatedAt                time.Time          `json:"created_at"`
	UpdatedAt                time.Time          `json:"updated_at"`
}

type ProcessConfigValues struct {
	Name                     string  `json:"name"`
	MinFermentationTempC     float64 `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64 `json:"max_fermentation_temp_c"`
	MinimumDays              int     `json:"minimum_days"`
	MaximumDays              int     `json:"maximum_days"`
	ConditioningObservations int     `json:"conditioning_observations"`
	CompletionObservations   int     `json:"completion_observations"`
}

type ProcessConfigBinding struct {
	ID                       string    `json:"id"`
	Key                      string    `json:"key"`
	ConfigVersion            int       `json:"config_version"`
	Name                     string    `json:"name"`
	MinFermentationTempC     float64   `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64   `json:"max_fermentation_temp_c"`
	MinimumDays              int       `json:"minimum_days"`
	MaximumDays              int       `json:"maximum_days"`
	ConditioningObservations int       `json:"conditioning_observations"`
	CompletionObservations   int       `json:"completion_observations"`
	EffectiveAt              time.Time `json:"effective_at"`
	PublishedAt              time.Time `json:"published_at"`
	StatusAtBinding          string    `json:"status_at_binding"`
	BoundAt                  time.Time `json:"bound_at"`
}

type ProcessSelectionDecision struct {
	Key                      string                          `json:"key"`
	EvaluatedAt              time.Time                       `json:"evaluated_at"`
	CandidatesConsidered     int                             `json:"candidates_considered"`
	SelectedConfigID         string                          `json:"selected_config_id,omitempty"`
	SelectedConfigVersion    int                             `json:"selected_config_version,omitempty"`
	Reason                   string                          `json:"reason,omitempty"`
	Basis                    string                          `json:"basis"`
	ConsideredConfigurations []ProcessSelectionConsideration `json:"considered_configurations"`
}

type ProcessSelectionConsideration struct {
	ID            string     `json:"id"`
	ConfigVersion int        `json:"config_version"`
	State         string     `json:"state"`
	EffectiveAt   *time.Time `json:"effective_at,omitempty"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	WithdrawnAt   *time.Time `json:"withdrawn_at,omitempty"`
	Eligible      bool       `json:"eligible"`
	Reason        string     `json:"reason,omitempty"`
}

func (c ProcessConfig) Clone() ProcessConfig {
	cloned := c
	cloned.EffectiveAt = cloneTimePointer(c.EffectiveAt)
	cloned.PublishedAt = cloneTimePointer(c.PublishedAt)
	cloned.WithdrawnAt = cloneTimePointer(c.WithdrawnAt)
	return cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}

func (c *ProcessConfig) Normalize() {
	c.Key = NormalizeName(c.Key)
	c.Name = NormalizeName(c.Name)
}

func (c ProcessConfig) Validate() error {
	if err := ValidateIdentifier(c.Key, "key"); err != nil {
		return err
	}
	if c.ConfigVersion < 1 {
		return FieldError(CodeInvalidInput, "config_version", "must be at least 1")
	}
	if c.Name == "" {
		return FieldError(CodeInvalidInput, "name", "is required")
	}
	if len([]rune(c.Name)) > 120 {
		return FieldError(CodeInvalidInput, "name", "must not exceed 120 characters")
	}
	if c.MinFermentationTempC < 5 || c.MinFermentationTempC > 40 {
		return FieldError(CodeInvalidInput, "min_fermentation_temp_c",
			"must be between 5 and 40 Celsius")
	}
	if c.MaxFermentationTempC < 5 || c.MaxFermentationTempC > 40 {
		return FieldError(CodeInvalidInput, "max_fermentation_temp_c",
			"must be between 5 and 40 Celsius")
	}
	if c.MinFermentationTempC > c.MaxFermentationTempC {
		return FieldError(CodeInvalidInput, "max_fermentation_temp_c",
			"must not be below the minimum fermentation temperature")
	}
	if c.MinimumDays < 0 || c.MinimumDays > 120 {
		return FieldError(CodeInvalidInput, "minimum_days",
			"must be between 0 and 120")
	}
	if c.MaximumDays < c.MinimumDays || c.MaximumDays < 1 || c.MaximumDays > 120 {
		return FieldError(CodeInvalidInput, "maximum_days",
			"must be between 1 and 120 and not below minimum days")
	}
	if c.ConditioningObservations < 1 || c.ConditioningObservations > 500 {
		return FieldError(CodeInvalidInput, "conditioning_observations",
			"must be between 1 and 500")
	}
	if c.CompletionObservations < c.ConditioningObservations ||
		c.CompletionObservations > 500 {
		return FieldError(CodeInvalidInput, "completion_observations",
			"must be between the conditioning threshold and 500")
	}
	switch c.State {
	case ProcessConfigDraft:
		if c.PublishedAt != nil || c.EffectiveAt != nil || c.WithdrawnAt != nil {
			return FieldError(CodeInvalidInput, "state", "draft cannot carry publication timestamps")
		}
	case ProcessConfigPublished:
		if c.PublishedAt == nil || c.EffectiveAt == nil || c.WithdrawnAt != nil {
			return FieldError(CodeInvalidInput, "state", "published configuration requires publication and effective timestamps")
		}
		if c.EffectiveAt.IsZero() || c.PublishedAt.IsZero() {
			return FieldError(CodeInvalidInput, "effective_at", "must not be the zero timestamp")
		}
	case ProcessConfigWithdrawn:
		if c.PublishedAt == nil || c.EffectiveAt == nil || c.WithdrawnAt == nil {
			return FieldError(CodeInvalidInput, "state", "withdrawn configuration requires its publication and withdrawal timestamps")
		}
		if c.EffectiveAt.IsZero() || c.PublishedAt.IsZero() || c.WithdrawnAt.IsZero() {
			return FieldError(CodeInvalidInput, "state", "withdrawn configuration timestamps must be set")
		}
	default:
		return FieldError(CodeInvalidInput, "state", "must be draft, published, or withdrawn")
	}
	return nil
}

func (c *ProcessConfig) Update(values ProcessConfigValues, now time.Time) error {
	if c.State != ProcessConfigDraft {
		return NewError(CodeProcessConfigState, "only a draft configuration can be changed")
	}
	candidate := *c
	candidate.Name = NormalizeName(values.Name)
	candidate.MinFermentationTempC = values.MinFermentationTempC
	candidate.MaxFermentationTempC = values.MaxFermentationTempC
	candidate.MinimumDays = values.MinimumDays
	candidate.MaximumDays = values.MaximumDays
	candidate.ConditioningObservations = values.ConditioningObservations
	candidate.CompletionObservations = values.CompletionObservations
	candidate.State = ProcessConfigDraft
	if err := candidate.Validate(); err != nil {
		return err
	}
	c.Name = candidate.Name
	c.MinFermentationTempC = candidate.MinFermentationTempC
	c.MaxFermentationTempC = candidate.MaxFermentationTempC
	c.MinimumDays = candidate.MinimumDays
	c.MaximumDays = candidate.MaximumDays
	c.ConditioningObservations = candidate.ConditioningObservations
	c.CompletionObservations = candidate.CompletionObservations
	c.UpdatedAt = now.UTC()
	c.Version++
	return nil
}

func (c *ProcessConfig) Publish(effectiveAt time.Time, now time.Time) error {
	if c.State != ProcessConfigDraft {
		return NewError(CodeProcessConfigState, "only a draft configuration can be published")
	}
	stamp := now.UTC()
	effective := effectiveAt.UTC()
	c.State = ProcessConfigPublished
	c.PublishedAt = &stamp
	c.EffectiveAt = &effective
	c.WithdrawnAt = nil
	c.UpdatedAt = stamp
	c.Version++
	return nil
}

func (c *ProcessConfig) Withdraw(now time.Time) error {
	if c.State != ProcessConfigPublished {
		return NewError(CodeProcessConfigState, "only a published configuration can be withdrawn")
	}
	stamp := now.UTC()
	c.State = ProcessConfigWithdrawn
	c.WithdrawnAt = &stamp
	c.UpdatedAt = stamp
	c.Version++
	return nil
}

func (c ProcessConfig) Binding(boundAt time.Time) ProcessConfigBinding {
	stamp := boundAt.UTC()
	return ProcessConfigBinding{
		ID:                       c.ID,
		Key:                      c.Key,
		ConfigVersion:            c.ConfigVersion,
		Name:                     c.Name,
		MinFermentationTempC:     c.MinFermentationTempC,
		MaxFermentationTempC:     c.MaxFermentationTempC,
		MinimumDays:              c.MinimumDays,
		MaximumDays:              c.MaximumDays,
		ConditioningObservations: c.ConditioningObservations,
		CompletionObservations:   c.CompletionObservations,
		EffectiveAt:              c.EffectiveAt.UTC(),
		PublishedAt:              c.PublishedAt.UTC(),
		StatusAtBinding:          string(ProcessConfigPublished),
		BoundAt:                  stamp,
	}
}

func SelectProcessConfig(configs map[string]ProcessConfig, key string, at time.Time) (ProcessConfig, ProcessSelectionDecision, error) {
	evaluated := at.UTC()
	family := make([]ProcessConfig, 0)
	for _, config := range configs {
		if config.Key == key {
			family = append(family, config)
		}
	}
	sort.Slice(family, func(i, j int) bool {
		left, right := family[i], family[j]
		leftEffective := processConfigEffectiveTime(left)
		rightEffective := processConfigEffectiveTime(right)
		if !leftEffective.Equal(rightEffective) {
			return leftEffective.After(rightEffective)
		}
		if left.ConfigVersion != right.ConfigVersion {
			return left.ConfigVersion > right.ConfigVersion
		}
		leftPublished, rightPublished := processConfigPublishedTime(left), processConfigPublishedTime(right)
		if !leftPublished.Equal(rightPublished) {
			return leftPublished.After(rightPublished)
		}
		return left.ID > right.ID
	})

	considerations := make([]ProcessSelectionConsideration, 0, len(family))
	var selected *ProcessConfig
	candidates := 0
	for _, config := range family {
		consideration := ProcessSelectionConsideration{
			ID:            config.ID,
			ConfigVersion: config.ConfigVersion,
			State:         string(config.State),
			EffectiveAt:   cloneTimePointer(config.EffectiveAt),
			PublishedAt:   cloneTimePointer(config.PublishedAt),
			WithdrawnAt:   cloneTimePointer(config.WithdrawnAt),
		}
		switch {
		case config.State != ProcessConfigPublished:
			consideration.Reason = "not currently published"
		case config.EffectiveAt == nil || config.PublishedAt == nil:
			consideration.Reason = "missing publication timestamps"
		case config.EffectiveAt.UTC().After(evaluated):
			consideration.Reason = "effective time is after the evaluated run start"
		default:
			consideration.Eligible = true
			consideration.Reason = "eligible published and effective configuration"
			candidates++
			current := config
			if selected == nil || configPrecedes(*selected, current) {
				selected = &current
			}
		}
		considerations = append(considerations, consideration)
	}

	decision := ProcessSelectionDecision{
		Key:                      key,
		EvaluatedAt:              evaluated,
		CandidatesConsidered:     candidates,
		Basis:                    ProcessSelectionBasis,
		ConsideredConfigurations: considerations,
	}
	if selected != nil {
		decision.SelectedConfigID = selected.ID
		decision.SelectedConfigVersion = selected.ConfigVersion
		decision.Reason = "selected the first eligible version in deterministic precedence order"
		return selected.Clone(), decision, nil
	}
	if len(family) == 0 {
		return ProcessConfig{}, decision, NewError(CodeProcessConfigNotFound,
			"process configuration family was not found")
	}
	return ProcessConfig{}, decision, NewError(CodeProcessConfigUnavailable,
		processConfigUnavailableMessage(considerations, evaluated))
}

func processConfigEffectiveTime(config ProcessConfig) time.Time {
	if config.EffectiveAt == nil {
		return time.Time{}
	}
	return config.EffectiveAt.UTC()
}

func processConfigPublishedTime(config ProcessConfig) time.Time {
	if config.PublishedAt == nil {
		return time.Time{}
	}
	return config.PublishedAt.UTC()
}

func processConfigUnavailableMessage(considerations []ProcessSelectionConsideration, evaluated time.Time) string {
	reasons := make([]string, 0, len(considerations))
	for _, consideration := range considerations {
		if consideration.Eligible {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("v%d (%s)", consideration.ConfigVersion, consideration.Reason))
	}
	if len(reasons) == 0 {
		return "no process configuration version is available at run start"
	}
	return "no effective published configuration at " + evaluated.Format(time.RFC3339Nano) +
		": " + strings.Join(reasons, "; ")
}

func configPrecedes(current, candidate ProcessConfig) bool {
	currentEffective := current.EffectiveAt.UTC()
	candidateEffective := candidate.EffectiveAt.UTC()
	if !candidateEffective.Equal(currentEffective) {
		return candidateEffective.After(currentEffective)
	}
	if candidate.ConfigVersion != current.ConfigVersion {
		return candidate.ConfigVersion > current.ConfigVersion
	}
	if !candidate.PublishedAt.UTC().Equal(current.PublishedAt.UTC()) {
		return candidate.PublishedAt.UTC().After(current.PublishedAt.UTC())
	}
	return candidate.ID > current.ID
}

func ProcessConfigFamilyHasPublished(configs map[string]ProcessConfig, key string) bool {
	for _, config := range configs {
		if config.Key == key && config.State == ProcessConfigPublished {
			return true
		}
	}
	return false
}
