package workflowcheck

import (
	"fmt"
	"strings"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func processConfigurationVersioning(api *client) error {
	api.http.Timeout = 10 * time.Second
	key := "seasonal-pale"
	name := "Seasonal Formula"

	v1, err := createPublishedProcessConfig(api, key, "Seasonal Pale v1",
		18.0, 20.0, time.Now().UTC().Add(-time.Minute), 3)
	if err != nil {
		return err
	}
	formula, err := createConfiguredApprovedFormula(api, name, key)
	if err != nil {
		return err
	}
	vesselA, err := createCheckVessel(api, "VSL-301", 300)
	if err != nil {
		return err
	}
	vesselB, err := createCheckVessel(api, "VSL-302", 300)
	if err != nil {
		return err
	}
	startedA, err := createStartedRun(api, formula.ID, vesselA.ID, "RUN-CONFIG-A")
	if err != nil {
		return err
	}
	if startedA.Run.ProcessConfig == nil || startedA.Run.ProcessDecision == nil {
		return errInvalidWorkflow("run start did not record a process configuration binding")
	}
	if startedA.Run.ProcessConfig.ID != v1.ID ||
		startedA.Run.ProcessConfig.ConfigVersion != 1 ||
		startedA.Run.ProcessDecision.CandidatesConsidered != 1 ||
		startedA.Run.ProcessDecision.SelectedConfigID != v1.ID ||
		len(startedA.Run.ProcessDecision.ConsideredConfigurations) != 1 {
		return errInvalidWorkflow("first run did not bind the effective v1 process configuration")
	}

	switchAt := time.Now().UTC().Add(1100 * time.Millisecond)
	v2, err := createPublishedProcessConfig(api, key, "Seasonal Pale v2",
		16.0, 18.0, switchAt, 2)
	if err != nil {
		return err
	}
	if v2.ConfigVersion != 2 {
		return fmt.Errorf("new process draft was not assigned version 2")
	}

	withdrawResponse, err := api.call("POST", "/v1/process-configs/"+v1.ID+"/withdraw",
		nil, ptrInt64(v1.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(withdrawResponse, 200, "withdraw old process configuration"); err != nil {
		return err
	}

	futureRun, err := createReservedRun(api, formula.ID, vesselB.ID, "RUN-CONFIG-FUTURE")
	if err != nil {
		return err
	}
	beforeEffective, err := api.call("POST", "/v1/runs/"+futureRun.Run.ID+"/start",
		nil, ptrInt64(futureRun.Run.Version))
	if err != nil {
		return err
	}
	if err := requireCode(beforeEffective, 409, string(domain.CodeProcessConfigUnavailable),
		"start before next configuration is effective"); err != nil {
		return err
	}

	time.Sleep(time.Until(switchAt) + 100*time.Millisecond)
	startedB, err := startReservedRun(api, futureRun.Run.ID, futureRun.Run.Version)
	if err != nil {
		return err
	}
	if startedB.Run.ProcessConfig == nil || startedB.Run.ProcessDecision == nil {
		return errInvalidWorkflow("second run did not record a process configuration binding")
	}
	if startedB.Run.ProcessConfig.ID != v2.ID ||
		startedB.Run.ProcessConfig.ConfigVersion != 2 ||
		startedB.Run.ProcessConfig.MinFermentationTempC != 16.0 ||
		startedB.Run.ProcessConfig.MaxFermentationTempC != 18.0 ||
		startedB.Run.ProcessDecision.CandidatesConsidered != 1 ||
		startedB.Run.ProcessDecision.SelectedConfigID != v2.ID ||
		len(startedB.Run.ProcessDecision.ConsideredConfigurations) != 2 {
		return errInvalidWorkflow("second run did not bind effective v2 after v1 withdrawal")
	}
	if !strings.Contains(startedB.Run.ProcessDecision.Basis, "effective_at") {
		return errInvalidWorkflow("process decision did not explain its selection basis")
	}

	baseA := time.Now().UTC().Add(time.Second)
	if err := appendConfigObservations(api, startedA.Run.ID, startedA.Run.Version, baseA, 19.0); err != nil {
		return err
	}
	if err := appendConfigObservations(api, startedA.Run.ID, 0, baseA, 19.2); err != nil {
		return err
	}
	if err := appendConfigObservations(api, startedA.Run.ID, 0, baseA, 19.5); err != nil {
		return err
	}
	baseB := time.Now().UTC().Add(2 * time.Second)
	if err := appendConfigObservations(api, startedB.Run.ID, 0,
		baseB, 19.0); err == nil {
		return errInvalidWorkflow("v1 temperature was accepted after binding v2")
	}
	if err := appendConfigObservations(api, startedB.Run.ID, startedB.Run.Version,
		baseB, 17.0); err != nil {
		return err
	}
	if err := appendConfigObservations(api, startedB.Run.ID, 0,
		baseB, 17.5); err != nil {
		return err
	}

	conditionB, err := transitionRun(api, startedB.Run.ID, "conditioning", 0)
	if err != nil {
		return err
	}
	if conditionB.Run.State != domain.RunConditioning {
		return errInvalidWorkflow("v2 two-observation conditioning threshold was not applied")
	}
	completeB, err := transitionRun(api, startedB.Run.ID, "complete", conditionB.Run.Version)
	if err != nil {
		return err
	}
	if completeB.Run.State != domain.RunCompleted ||
		completeB.Run.ProcessConfig == nil ||
		completeB.Run.ProcessConfig.ConfigVersion != 2 {
		return errInvalidWorkflow("completion did not continue with the bound v2 configuration")
	}

	conditionA, err := transitionRun(api, startedA.Run.ID, "conditioning", 0)
	if err != nil {
		return err
	}
	completeA, err := transitionRun(api, startedA.Run.ID, "complete", conditionA.Run.Version)
	if err != nil {
		return err
	}
	if completeA.Run.State != domain.RunCompleted ||
		completeA.Run.ProcessConfig == nil ||
		completeA.Run.ProcessConfig.ID != v1.ID {
		return errInvalidWorkflow("older run was not judged and completed with v1")
	}
	return nil
}

func createPublishedProcessConfig(
	api *client,
	key string,
	name string,
	minTemp float64,
	maxTemp float64,
	effectiveAt time.Time,
	completionObservations int,
) (processConfigView, error) {
	payload := createProcessConfigPayload(key, name, minTemp, maxTemp)
	payload["completion_observations"] = completionObservations
	createResponse, err := api.call("POST", "/v1/process-configs", payload, nil)
	if err != nil {
		return processConfigView{}, err
	}
	if err := requireStatus(createResponse, 201, "create seasonal process configuration"); err != nil {
		return processConfigView{}, err
	}
	var config processConfigView
	if err := createResponse.decode(&config); err != nil {
		return processConfigView{}, err
	}
	publishResponse, err := api.call("POST", "/v1/process-configs/"+config.ID+"/publish",
		map[string]any{"effective_at": effectiveAt}, ptrInt64(1))
	if err != nil {
		return processConfigView{}, err
	}
	if err := requireStatus(publishResponse, 200, "publish seasonal process configuration"); err != nil {
		return processConfigView{}, err
	}
	if err := publishResponse.decode(&config); err != nil {
		return processConfigView{}, err
	}
	return config, nil
}

func createConfiguredApprovedFormula(api *client, name, key string) (formulaView, error) {
	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayloadWithKey(name, key), nil)
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(createdResponse, 201, "create seasonal formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := createdResponse.decode(&formula); err != nil {
		return formulaView{}, err
	}
	if _, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1)); err != nil {
		return formulaView{}, err
	}
	selectResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/process-config",
		map[string]any{"process_config_key": key}, ptrInt64(2))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(selectResponse, 200, "select seasonal process configuration"); err != nil {
		return formulaView{}, err
	}
	approvedResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(3))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(approvedResponse, 200, "approve seasonal formula"); err != nil {
		return formulaView{}, err
	}
	if err := approvedResponse.decode(&formula); err != nil {
		return formulaView{}, err
	}
	return formula, nil
}

func createReservedRun(api *client, formulaID, vesselID, code string) (pairView, error) {
	runResponse, err := api.call("POST", "/v1/runs", createRunPayload(code, formulaID), nil)
	if err != nil {
		return pairView{}, err
	}
	if err := requireStatus(runResponse, 201, "create config run"); err != nil {
		return pairView{}, err
	}
	var run runView
	if err := runResponse.decode(&run); err != nil {
		return pairView{}, err
	}
	reserveResponse, err := api.call("POST", "/v1/runs/"+run.ID+"/reserve",
		map[string]any{"vessel_id": vesselID}, ptrInt64(run.Version))
	if err != nil {
		return pairView{}, err
	}
	if err := requireStatus(reserveResponse, 200, "reserve config vessel"); err != nil {
		return pairView{}, err
	}
	var reserved pairView
	if err := reserveResponse.decode(&reserved); err != nil {
		return pairView{}, err
	}
	return reserved, nil
}

func startReservedRun(api *client, runID string, version int64) (pairView, error) {
	var expected *int64
	if version > 0 {
		expected = ptrInt64(version)
	}
	response, err := api.call("POST", "/v1/runs/"+runID+"/start", nil, expected)
	if err != nil {
		return pairView{}, err
	}
	if err := requireStatus(response, 200, "start config run"); err != nil {
		return pairView{}, err
	}
	var pair pairView
	if err := response.decode(&pair); err != nil {
		return pairView{}, err
	}
	return pair, nil
}

func appendConfigObservations(
	api *client,
	runID string,
	version int64,
	base time.Time,
	temperature float64,
) error {
	runResponse, err := api.call("GET", "/v1/runs/"+runID, nil, nil)
	if err != nil {
		return err
	}
	var run runView
	if err := runResponse.decode(&run); err != nil {
		return err
	}
	sequence := len(run.Observations) + 1
	var expected *int64
	if version > 0 {
		expected = ptrInt64(version)
	}
	response, err := api.call("POST", "/v1/runs/"+runID+"/observations", map[string]any{
		"sequence":      sequence,
		"gravity":       1.050 - float64(sequence)*0.010,
		"temperature_c": temperature,
		"observed_at":   base.Add(time.Duration(sequence) * time.Second),
	}, expected)
	if err != nil {
		return err
	}
	if response.status != 200 {
		return fmt.Errorf("append config observation: status %d: %s",
			response.status, response.body)
	}
	return nil
}

func transitionRun(api *client, runID, action string, version int64) (pairView, error) {
	if version == 0 {
		readResponse, err := api.call("GET", "/v1/runs/"+runID, nil, nil)
		if err != nil {
			return pairView{}, err
		}
		var run runView
		if err := readResponse.decode(&run); err != nil {
			return pairView{}, err
		}
		version = run.Version
	}
	response, err := api.call("POST", "/v1/runs/"+runID+"/"+action, nil, ptrInt64(version))
	if err != nil {
		return pairView{}, err
	}
	if response.status != 200 {
		return pairView{}, fmt.Errorf("%s run: status %d: %s", action, response.status, response.body)
	}
	if action == "complete" {
		var pair pairView
		if err := response.decode(&pair); err != nil {
			return pairView{}, err
		}
		return pair, nil
	}
	var run runView
	if err := response.decode(&run); err != nil {
		return pairView{}, err
	}
	return pairView{Run: run}, nil
}
