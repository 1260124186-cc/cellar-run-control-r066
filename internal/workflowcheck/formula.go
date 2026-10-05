package workflowcheck

import (
	"fmt"
	"strings"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func formulaApproval(api *client) error {
	key := "current-pale"
	if err := ensurePublishedProcessConfig(api, key, "Current Pale Process",
		18.0, 22.0, time.Now().UTC().Add(-time.Minute)); err != nil {
		return err
	}
	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayloadWithKey("Current Pale", key), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createdResponse, 201, "create formula"); err != nil {
		return err
	}
	var formula formulaView
	if err := createdResponse.decode(&formula); err != nil {
		return err
	}
	if formula.State != domain.FormulaDraft || formula.Version != 1 {
		return fmt.Errorf("created formula has unexpected state or version")
	}

	premature, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireCode(premature, 400, string(domain.CodeInvalidInput),
		"approve without steps"); err != nil {
		return err
	}

	updatedResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(updatedResponse, 200, "set formula steps"); err != nil {
		return err
	}
	if err := updatedResponse.decode(&formula); err != nil {
		return err
	}
	if formula.Version != 2 || len(formula.Steps) != 3 {
		return fmt.Errorf("formula steps were not accepted as one revision")
	}

	selectedResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/process-config",
		map[string]any{"process_config_key": key}, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(selectedResponse, 200, "select process configuration"); err != nil {
		return err
	}

	duplicate, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("  current   pale  "), nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeFormulaNameConflict),
		"create duplicate formula"); err != nil {
		return err
	}

	approvedResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(3))
	if err != nil {
		return err
	}
	if err := requireStatus(approvedResponse, 200, "approve formula"); err != nil {
		return err
	}
	if err := approvedResponse.decode(&formula); err != nil {
		return err
	}
	if formula.State != domain.FormulaApproved || formula.Version != 4 ||
		formula.ProcessConfigKey != key {
		return fmt.Errorf("approved formula has unexpected state, version, or process configuration")
	}

	readResponse, err := api.call("GET", "/v1/formulas/"+formula.ID, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(readResponse, 200, "read formula"); err != nil {
		return err
	}
	var readBack formulaView
	if err := readResponse.decode(&readBack); err != nil {
		return err
	}
	if readBack.State != domain.FormulaApproved || len(readBack.Steps) != 3 {
		return fmt.Errorf("approved formula was not readable")
	}

	repeated, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(4))
	if err != nil {
		return err
	}
	if err := requireCode(repeated, 409, string(domain.CodeFormulaState),
		"approve formula twice"); err != nil {
		return err
	}
	return nil
}

func ensureApprovedFormula(api *client, name string) (formulaView, error) {
	key := processKeyForName(name)
	if err := ensurePublishedProcessConfig(api, key, name+" Process", 18.0, 22.0, time.Now().UTC().Add(-time.Minute)); err != nil {
		return formulaView{}, err
	}
	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayloadWithKey(name, key), nil)
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(createdResponse, 201, "create setup formula"); err != nil {
		return formulaView{}, err
	}
	var formula formulaView
	if err := createdResponse.decode(&formula); err != nil {
		return formulaView{}, err
	}
	stepsResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(stepsResponse, 200, "set setup steps"); err != nil {
		return formulaView{}, err
	}
	selectResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/process-config",
		map[string]any{"process_config_key": key}, ptrInt64(2))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(selectResponse, 200, "select setup process configuration"); err != nil {
		return formulaView{}, err
	}
	approveResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(3))
	if err != nil {
		return formulaView{}, err
	}
	if err := requireStatus(approveResponse, 200, "approve setup formula"); err != nil {
		return formulaView{}, err
	}
	if err := approveResponse.decode(&formula); err != nil {
		return formulaView{}, err
	}
	return formula, nil
}

func processKeyForName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

func ensurePublishedProcessConfig(
	api *client,
	key string,
	name string,
	minTemp float64,
	maxTemp float64,
	effectiveAt time.Time,
) error {
	createResponse, err := api.call("POST", "/v1/process-configs",
		createProcessConfigPayload(key, name, minTemp, maxTemp), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createResponse, 201, "create process configuration"); err != nil {
		return err
	}
	var config processConfigView
	if err := createResponse.decode(&config); err != nil {
		return err
	}
	publishResponse, err := api.call("POST", "/v1/process-configs/"+config.ID+"/publish",
		map[string]any{"effective_at": effectiveAt}, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(publishResponse, 200, "publish process configuration"); err != nil {
		return err
	}
	return nil
}

func ptrInt64(value int64) *int64 {
	return &value
}
