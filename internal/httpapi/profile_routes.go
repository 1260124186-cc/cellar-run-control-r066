package httpapi

import (
	"net/http"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type profileParametersRequest struct {
	MinFermentationTempC       float64 `json:"min_fermentation_temp_c"`
	MaxFermentationTempC       float64 `json:"max_fermentation_temp_c"`
	MinFermentDays             int     `json:"min_ferment_days"`
	MaxFermentDays             int     `json:"max_ferment_days"`
	MinObservationsToCondition int     `json:"min_observations_to_condition"`
	MinObservationsToComplete  int     `json:"min_observations_to_complete"`
}

func (r profileParametersRequest) parameters() domain.ProfileParameters {
	return domain.ProfileParameters{
		MinFermentationTempC:       r.MinFermentationTempC,
		MaxFermentationTempC:       r.MaxFermentationTempC,
		MinFermentDays:             r.MinFermentDays,
		MaxFermentDays:             r.MaxFermentDays,
		MinObservationsToCondition: r.MinObservationsToCondition,
		MinObservationsToComplete:  r.MinObservationsToComplete,
	}
}

type createProfileRequest struct {
	Name string `json:"name"`
	profileParametersRequest
	EffectiveAt time.Time `json:"effective_at"`
}

type updateProfileRequest struct {
	profileParametersRequest
	EffectiveAt time.Time `json:"effective_at"`
}

type reviseProfileRequest struct {
	EffectiveAt time.Time `json:"effective_at"`
}

func registerProfileRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/profiles", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createProfileRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		profile, err := app.CreateProfile(service.CreateProfileInput{
			Name:        body.Name,
			Parameters:  body.parameters(),
			EffectiveAt: body.EffectiveAt,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, profile)
	})

	mux.HandleFunc("GET /v1/profiles", func(response http.ResponseWriter, _ *http.Request) {
		profiles, err := app.ListProfiles()
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"profiles": profiles})
	})

	mux.HandleFunc("GET /v1/profiles/{id}", func(response http.ResponseWriter, request *http.Request) {
		profile, err := app.GetProfile(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, profile)
	})

	mux.HandleFunc("PUT /v1/profiles/{id}", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[updateProfileRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		profile, err := app.UpdateProfile(request.PathValue("id"), service.UpdateProfileInput{
			ExpectedVersion: expected,
			Parameters:      body.parameters(),
			EffectiveAt:     body.EffectiveAt,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, profile)
	})

	mux.HandleFunc("POST /v1/profiles/{id}/publish", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		profile, err := app.PublishProfile(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, profile)
	})

	mux.HandleFunc("POST /v1/profiles/{id}/revise", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[reviseProfileRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		profile, err := app.ReviseProfile(request.PathValue("id"), service.ReviseProfileInput{
			ExpectedVersion: expected,
			EffectiveAt:     body.EffectiveAt,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, profile)
	})

	mux.HandleFunc("POST /v1/profiles/{id}/withdraw", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		profile, err := app.WithdrawProfile(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, profile)
	})
}
