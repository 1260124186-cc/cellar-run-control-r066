package httpapi

import (
	"net/http"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createProcessConfigRequest struct {
	Key                      string  `json:"key"`
	Name                     string  `json:"name"`
	MinFermentationTempC     float64 `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64 `json:"max_fermentation_temp_c"`
	MinimumDays              int     `json:"minimum_days"`
	MaximumDays              int     `json:"maximum_days"`
	ConditioningObservations int     `json:"conditioning_observations"`
	CompletionObservations   int     `json:"completion_observations"`
}

type updateProcessConfigRequest struct {
	Name                     string  `json:"name"`
	MinFermentationTempC     float64 `json:"min_fermentation_temp_c"`
	MaxFermentationTempC     float64 `json:"max_fermentation_temp_c"`
	MinimumDays              int     `json:"minimum_days"`
	MaximumDays              int     `json:"maximum_days"`
	ConditioningObservations int     `json:"conditioning_observations"`
	CompletionObservations   int     `json:"completion_observations"`
}

type publishProcessConfigRequest struct {
	EffectiveAt time.Time `json:"effective_at"`
}

func registerProcessConfigRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/process-configs", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createProcessConfigRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		config, err := app.CreateProcessConfig(service.CreateProcessConfigInput{
			Key:                      body.Key,
			Name:                     body.Name,
			MinFermentationTempC:     body.MinFermentationTempC,
			MaxFermentationTempC:     body.MaxFermentationTempC,
			MinimumDays:              body.MinimumDays,
			MaximumDays:              body.MaximumDays,
			ConditioningObservations: body.ConditioningObservations,
			CompletionObservations:   body.CompletionObservations,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, config)
	})

	mux.HandleFunc("GET /v1/process-configs", func(response http.ResponseWriter, _ *http.Request) {
		configs, err := app.ListProcessConfigs()
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"process_configs": configs})
	})

	mux.HandleFunc("GET /v1/process-configs/{id}", func(response http.ResponseWriter, request *http.Request) {
		config, err := app.GetProcessConfig(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, config)
	})

	mux.HandleFunc("PUT /v1/process-configs/{id}", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[updateProcessConfigRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		config, err := app.UpdateProcessConfig(request.PathValue("id"), service.UpdateProcessConfigInput{
			ExpectedVersion:          expected,
			Name:                     body.Name,
			MinFermentationTempC:     body.MinFermentationTempC,
			MaxFermentationTempC:     body.MaxFermentationTempC,
			MinimumDays:              body.MinimumDays,
			MaximumDays:              body.MaximumDays,
			ConditioningObservations: body.ConditioningObservations,
			CompletionObservations:   body.CompletionObservations,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, config)
	})

	mux.HandleFunc("POST /v1/process-configs/{id}/publish", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[publishProcessConfigRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		config, err := app.PublishProcessConfig(request.PathValue("id"), service.PublishProcessConfigInput{
			ExpectedVersion: expected,
			EffectiveAt:     body.EffectiveAt,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, config)
	})

	mux.HandleFunc("POST /v1/process-configs/{id}/withdraw", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		config, err := app.WithdrawProcessConfig(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, config)
	})
}
