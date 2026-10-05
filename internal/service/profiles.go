package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateProfile(input CreateProfileInput) (domain.ProcessProfile, error) {
	now := s.clock.Now()
	profile := domain.ProcessProfile{
		Name:        input.Name,
		Version:     1,
		Parameters:  input.Parameters,
		EffectiveAt: input.EffectiveAt.UTC(),
		State:       domain.ProfileDraft,
		Revision:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	profile.Normalize()
	if err := profile.Validate(); err != nil {
		return domain.ProcessProfile{}, err
	}
	id, err := generateEntityID("profile")
	if err != nil {
		return domain.ProcessProfile{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	profile.ID = id
	nameKey := domain.NameKey(profile.Name)

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if _, exists := snapshot.ProfileNameIndex[nameKey]; exists {
			return domain.NewError(domain.CodeProfileNameConflict,
				"a process profile with the same normalized name already exists; revise it to publish a new version")
		}
		snapshot.Profiles[profile.ID] = profile
		snapshot.ProfileNameIndex[nameKey] = profile.ID
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return profile.Clone(), nil
}

func (s *Service) UpdateProfile(
	id string,
	input UpdateProfileInput,
) (domain.ProcessProfile, error) {
	var updated domain.ProcessProfile
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		profile, ok := snapshot.Profiles[id]
		if !ok {
			return domain.NewError(domain.CodeProfileNotFound, "process profile was not found")
		}
		if err := s.checkExpected(profile.Revision, input.ExpectedVersion); err != nil {
			return err
		}
		if err := profile.Update(input.Parameters, input.EffectiveAt, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Profiles[id] = profile
		updated = profile.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return updated, nil
}

func (s *Service) PublishProfile(id string, input VersionInput) (domain.ProcessProfile, error) {
	var updated domain.ProcessProfile
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		profile, ok := snapshot.Profiles[id]
		if !ok {
			return domain.NewError(domain.CodeProfileNotFound, "process profile was not found")
		}
		if err := s.checkExpected(profile.Revision, input.ExpectedVersion); err != nil {
			return err
		}
		if err := profile.Publish(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Profiles[id] = profile
		updated = profile.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return updated, nil
}

// ReviseProfile forks a published profile version into a new draft carrying
// the next publication version of the family. The source version is left
// untouched so runs bound to it keep their standard.
func (s *Service) ReviseProfile(
	id string,
	input ReviseProfileInput,
) (domain.ProcessProfile, error) {
	if input.EffectiveAt.IsZero() {
		return domain.ProcessProfile{}, domain.FieldError(domain.CodeInvalidInput,
			"effective_at", "is required")
	}
	draftID, err := generateEntityID("profile")
	if err != nil {
		return domain.ProcessProfile{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	var created domain.ProcessProfile
	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		source, ok := snapshot.Profiles[id]
		if !ok {
			return domain.NewError(domain.CodeProfileNotFound, "process profile was not found")
		}
		if err := s.checkExpected(source.Revision, input.ExpectedVersion); err != nil {
			return err
		}
		if source.State != domain.ProfilePublished {
			return domain.NewError(domain.CodeProfileState,
				"only a published profile can be revised")
		}
		nameKey := domain.NameKey(source.Name)
		maxVersion := 0
		for _, candidate := range snapshot.Profiles {
			if domain.NameKey(candidate.Name) == nameKey && candidate.Version > maxVersion {
				maxVersion = candidate.Version
			}
		}
		now := s.clock.Now()
		draft := domain.ProcessProfile{
			ID:          draftID,
			Name:        source.Name,
			Version:     maxVersion + 1,
			Parameters:  source.Parameters,
			EffectiveAt: input.EffectiveAt.UTC(),
			State:       domain.ProfileDraft,
			Revision:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := draft.Validate(); err != nil {
			return err
		}
		snapshot.Profiles[draft.ID] = draft
		created = draft.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return created, nil
}

func (s *Service) WithdrawProfile(id string, input VersionInput) (domain.ProcessProfile, error) {
	var updated domain.ProcessProfile
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		profile, ok := snapshot.Profiles[id]
		if !ok {
			return domain.NewError(domain.CodeProfileNotFound, "process profile was not found")
		}
		if err := s.checkExpected(profile.Revision, input.ExpectedVersion); err != nil {
			return err
		}
		if err := profile.Withdraw(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Profiles[id] = profile
		updated = profile.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return updated, nil
}

func (s *Service) GetProfile(id string) (domain.ProcessProfile, error) {
	var found domain.ProcessProfile
	err := s.store.View(func(snapshot domain.Snapshot) error {
		profile, ok := snapshot.Profiles[id]
		if !ok {
			return domain.NewError(domain.CodeProfileNotFound, "process profile was not found")
		}
		found = profile.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessProfile{}, err
	}
	return found, nil
}

func (s *Service) ListProfiles() ([]domain.ProcessProfile, error) {
	var profiles []domain.ProcessProfile
	err := s.store.View(func(snapshot domain.Snapshot) error {
		profiles = make([]domain.ProcessProfile, 0, len(snapshot.Profiles))
		for _, profile := range snapshot.Profiles {
			profiles = append(profiles, profile.Clone())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(profiles, func(i, j int) bool {
		leftKey := domain.NameKey(profiles[i].Name)
		rightKey := domain.NameKey(profiles[j].Name)
		if leftKey == rightKey {
			return profiles[i].Version < profiles[j].Version
		}
		return leftKey < rightKey
	})
	return profiles, nil
}
