package domain

import (
	"fmt"
	"time"
)

type ProfileState string

const (
	ProfileDraft     ProfileState = "draft"
	ProfilePublished ProfileState = "published"
	ProfileWithdrawn ProfileState = "withdrawn"
)

// BindingReasonRunStart explains why a run adopted its profile version: it was
// the latest published version already effective when fermentation started.
const BindingReasonRunStart = "latest_effective_at_run_start"

// ProfileParameters carries the seasonal and style-specific expectations a
// run is judged against: fermentation temperature band, fermentation
// duration window, and observation thresholds.
type ProfileParameters struct {
	MinFermentationTempC       float64 `json:"min_fermentation_temp_c"`
	MaxFermentationTempC       float64 `json:"max_fermentation_temp_c"`
	MinFermentDays             int     `json:"min_ferment_days"`
	MaxFermentDays             int     `json:"max_ferment_days"`
	MinObservationsToCondition int     `json:"min_observations_to_condition"`
	MinObservationsToComplete  int     `json:"min_observations_to_complete"`
}

func (p ProfileParameters) Validate() error {
	if p.MinFermentationTempC < -5 || p.MinFermentationTempC > 60 {
		return FieldError(CodeInvalidInput, "min_fermentation_temp_c",
			"must be between -5 and 60 Celsius")
	}
	if p.MaxFermentationTempC < p.MinFermentationTempC || p.MaxFermentationTempC > 60 {
		return FieldError(CodeInvalidInput, "max_fermentation_temp_c",
			"must be between the minimum and 60 Celsius")
	}
	if p.MinFermentDays < 0 || p.MinFermentDays > 120 {
		return FieldError(CodeInvalidInput, "min_ferment_days",
			"must be between 0 and 120")
	}
	if p.MaxFermentDays < p.MinFermentDays || p.MaxFermentDays > 120 {
		return FieldError(CodeInvalidInput, "max_ferment_days",
			"must be between the minimum and 120")
	}
	if p.MinObservationsToCondition < 1 || p.MinObservationsToCondition > 500 {
		return FieldError(CodeInvalidInput, "min_observations_to_condition",
			"must be between 1 and 500")
	}
	if p.MinObservationsToComplete < p.MinObservationsToCondition ||
		p.MinObservationsToComplete > 500 {
		return FieldError(CodeInvalidInput, "min_observations_to_complete",
			"must be between the conditioning threshold and 500")
	}
	return nil
}

// DefaultProfileParameters matches the thresholds the service applied before
// versioned profiles existed. It is only a defensive fallback for runs that
// carry no binding.
func DefaultProfileParameters() ProfileParameters {
	return ProfileParameters{
		MinFermentationTempC:       -5,
		MaxFermentationTempC:       60,
		MinFermentDays:             0,
		MaxFermentDays:             120,
		MinObservationsToCondition: 2,
		MinObservationsToComplete:  3,
	}
}

// ProcessProfile is one immutable-after-publish version of a named process
// profile family. Version is the publication version inside the family;
// Revision is the optimistic-concurrency token checked by X-Expected-Version.
type ProcessProfile struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     int               `json:"version"`
	Parameters  ProfileParameters `json:"parameters"`
	EffectiveAt time.Time         `json:"effective_at"`
	State       ProfileState      `json:"state"`
	Revision    int64             `json:"revision"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	PublishedAt *time.Time        `json:"published_at,omitempty"`
	WithdrawnAt *time.Time        `json:"withdrawn_at,omitempty"`
}

func (p ProcessProfile) Clone() ProcessProfile {
	cloned := p
	if p.PublishedAt != nil {
		value := *p.PublishedAt
		cloned.PublishedAt = &value
	}
	if p.WithdrawnAt != nil {
		value := *p.WithdrawnAt
		cloned.WithdrawnAt = &value
	}
	return cloned
}

func (p *ProcessProfile) Normalize() {
	p.Name = NormalizeName(p.Name)
}

func (p ProcessProfile) Validate() error {
	if p.Name == "" {
		return FieldError(CodeInvalidInput, "name", "is required")
	}
	if len([]rune(p.Name)) > 120 {
		return FieldError(CodeInvalidInput, "name", "must not exceed 120 characters")
	}
	if p.Version < 1 {
		return FieldError(CodeInvalidInput, "version", "must be at least 1")
	}
	if p.EffectiveAt.IsZero() {
		return FieldError(CodeInvalidInput, "effective_at", "is required")
	}
	return p.Parameters.Validate()
}

func (p *ProcessProfile) Update(
	parameters ProfileParameters,
	effectiveAt time.Time,
	now time.Time,
) error {
	if p.State != ProfileDraft {
		return NewError(CodeProfileState, "only a draft profile can be edited")
	}
	candidate := ProcessProfile{
		Name:        p.Name,
		Version:     p.Version,
		Parameters:  parameters,
		EffectiveAt: effectiveAt.UTC(),
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	p.Parameters = parameters
	p.EffectiveAt = effectiveAt.UTC()
	p.UpdatedAt = now.UTC()
	p.Revision++
	return nil
}

func (p *ProcessProfile) Publish(now time.Time) error {
	if p.State != ProfileDraft {
		return NewError(CodeProfileState, "only a draft profile can be published")
	}
	stamp := now.UTC()
	p.State = ProfilePublished
	p.PublishedAt = &stamp
	p.UpdatedAt = stamp
	p.Revision++
	return nil
}

func (p *ProcessProfile) Withdraw(now time.Time) error {
	if p.State != ProfilePublished {
		return NewError(CodeProfileState, "only a published profile can be withdrawn")
	}
	stamp := now.UTC()
	p.State = ProfileWithdrawn
	p.WithdrawnAt = &stamp
	p.UpdatedAt = stamp
	p.Revision++
	return nil
}

// ProfileBinding records which profile version a run adopted, when, and why,
// together with a snapshot of the parameters that judge the run. The snapshot
// keeps completed and in-progress runs immune to later profile changes.
type ProfileBinding struct {
	ProfileID      string            `json:"profile_id"`
	ProfileName    string            `json:"profile_name"`
	ProfileVersion int               `json:"profile_version"`
	BoundAt        time.Time         `json:"bound_at"`
	Reason         string            `json:"reason"`
	Parameters     ProfileParameters `json:"parameters"`
}

func (b ProfileBinding) Describe() string {
	if b.ProfileName == "" {
		return "unversioned default parameters"
	}
	return fmt.Sprintf("process profile %q v%d", b.ProfileName, b.ProfileVersion)
}

// ResolveProfileBinding selects the profile version a run would adopt at the
// given moment: among published versions of the named family whose effective
// time has passed, the one with the latest effective time wins, with the
// higher publication version breaking ties. Withdrawn and not-yet-effective
// versions are never selected.
func ResolveProfileBinding(
	profiles []ProcessProfile,
	name string,
	at time.Time,
) (ProfileBinding, bool) {
	key := NameKey(name)
	if key == "" {
		return ProfileBinding{}, false
	}
	moment := at.UTC()
	var best *ProcessProfile
	for index := range profiles {
		candidate := profiles[index]
		if NameKey(candidate.Name) != key || candidate.State != ProfilePublished {
			continue
		}
		if candidate.EffectiveAt.After(moment) {
			continue
		}
		if best == nil ||
			candidate.EffectiveAt.After(best.EffectiveAt) ||
			(candidate.EffectiveAt.Equal(best.EffectiveAt) &&
				candidate.Version > best.Version) {
			selected := candidate
			best = &selected
		}
	}
	if best == nil {
		return ProfileBinding{}, false
	}
	return ProfileBinding{
		ProfileID:      best.ID,
		ProfileName:    best.Name,
		ProfileVersion: best.Version,
		BoundAt:        moment,
		Reason:         BindingReasonRunStart,
		Parameters:     best.Parameters,
	}, true
}
