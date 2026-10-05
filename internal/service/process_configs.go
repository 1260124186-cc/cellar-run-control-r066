package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateProcessConfig(input CreateProcessConfigInput) (domain.ProcessConfig, error) {
	now := s.clock.Now()
	key := domain.NormalizeName(input.Key)
	if err := domain.ValidateIdentifier(key, "key"); err != nil {
		return domain.ProcessConfig{}, err
	}
	config := domain.ProcessConfig{
		Key:                      key,
		Name:                     domain.NormalizeName(input.Name),
		MinFermentationTempC:     input.MinFermentationTempC,
		MaxFermentationTempC:     input.MaxFermentationTempC,
		MinimumDays:              input.MinimumDays,
		MaximumDays:              input.MaximumDays,
		ConditioningObservations: input.ConditioningObservations,
		CompletionObservations:   input.CompletionObservations,
		State:                    domain.ProcessConfigDraft,
		Version:                  1,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	id, err := generateEntityID("proc")
	if err != nil {
		return domain.ProcessConfig{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	config.ID = id

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if _, exists := snapshot.ProcessDraftIndex[config.Key]; exists {
			return domain.NewError(domain.CodeProcessConfigConflict,
				"this process configuration key already has an editable draft")
		}
		config.ConfigVersion = nextProcessConfigVersion(snapshot, config.Key)
		if err := config.Validate(); err != nil {
			return err
		}
		snapshot.ProcessConfigs[config.ID] = config.Clone()
		snapshot.ProcessDraftIndex[config.Key] = config.ID
		return nil
	})
	if err != nil {
		return domain.ProcessConfig{}, err
	}
	return config.Clone(), nil
}

func (s *Service) UpdateProcessConfig(id string, input UpdateProcessConfigInput) (domain.ProcessConfig, error) {
	var updated domain.ProcessConfig
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		config, ok := snapshot.ProcessConfigs[id]
		if !ok {
			return domain.NewError(domain.CodeProcessConfigNotFound, "process configuration was not found")
		}
		if err := s.checkExpected(config.Version, input.ExpectedVersion); err != nil {
			return err
		}
		values := domain.ProcessConfigValues{
			Name:                     input.Name,
			MinFermentationTempC:     input.MinFermentationTempC,
			MaxFermentationTempC:     input.MaxFermentationTempC,
			MinimumDays:              input.MinimumDays,
			MaximumDays:              input.MaximumDays,
			ConditioningObservations: input.ConditioningObservations,
			CompletionObservations:   input.CompletionObservations,
		}
		if err := config.Update(values, s.clock.Now()); err != nil {
			return err
		}
		snapshot.ProcessConfigs[id] = config.Clone()
		updated = config.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessConfig{}, err
	}
	return updated, nil
}

func (s *Service) PublishProcessConfig(id string, input PublishProcessConfigInput) (domain.ProcessConfig, error) {
	var updated domain.ProcessConfig
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		config, ok := snapshot.ProcessConfigs[id]
		if !ok {
			return domain.NewError(domain.CodeProcessConfigNotFound, "process configuration was not found")
		}
		if err := s.checkExpected(config.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := config.Publish(input.EffectiveAt, s.clock.Now()); err != nil {
			return err
		}
		if err := config.Validate(); err != nil {
			return err
		}
		delete(snapshot.ProcessDraftIndex, config.Key)
		snapshot.ProcessConfigs[id] = config.Clone()
		updated = config.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessConfig{}, err
	}
	return updated, nil
}

func (s *Service) WithdrawProcessConfig(id string, input VersionInput) (domain.ProcessConfig, error) {
	var updated domain.ProcessConfig
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		config, ok := snapshot.ProcessConfigs[id]
		if !ok {
			return domain.NewError(domain.CodeProcessConfigNotFound, "process configuration was not found")
		}
		if err := s.checkExpected(config.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := config.Withdraw(s.clock.Now()); err != nil {
			return err
		}
		snapshot.ProcessConfigs[id] = config.Clone()
		updated = config.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessConfig{}, err
	}
	return updated, nil
}

func (s *Service) GetProcessConfig(id string) (domain.ProcessConfig, error) {
	var found domain.ProcessConfig
	err := s.store.View(func(snapshot domain.Snapshot) error {
		config, ok := snapshot.ProcessConfigs[id]
		if !ok {
			return domain.NewError(domain.CodeProcessConfigNotFound, "process configuration was not found")
		}
		found = config.Clone()
		return nil
	})
	if err != nil {
		return domain.ProcessConfig{}, err
	}
	return found, nil
}

func (s *Service) ListProcessConfigs() ([]domain.ProcessConfig, error) {
	var configs []domain.ProcessConfig
	err := s.store.View(func(snapshot domain.Snapshot) error {
		configs = make([]domain.ProcessConfig, 0, len(snapshot.ProcessConfigs))
		for _, config := range snapshot.ProcessConfigs {
			configs = append(configs, config.Clone())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].Key != configs[j].Key {
			return configs[i].Key < configs[j].Key
		}
		if configs[i].ConfigVersion != configs[j].ConfigVersion {
			return configs[i].ConfigVersion < configs[j].ConfigVersion
		}
		return configs[i].ID < configs[j].ID
	})
	return configs, nil
}

func nextProcessConfigVersion(snapshot *domain.Snapshot, key string) int {
	highest := 0
	for _, config := range snapshot.ProcessConfigs {
		if config.Key == key && config.ConfigVersion > highest {
			highest = config.ConfigVersion
		}
	}
	return highest + 1
}
