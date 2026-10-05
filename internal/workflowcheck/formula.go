package workflowcheck

import (
	"fmt"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func formulaApproval(api *client) error {
	profile, err := ensurePublishedProfile(api, "Approval Profile")
	if err != nil {
		return err
	}

	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Current Pale", profile.Name), nil)
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
	if formula.ProfileName != profile.Name {
		return fmt.Errorf("created formula did not retain its process profile")
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

	duplicate, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("  current   pale  ", profile.Name), nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeFormulaNameConflict),
		"create duplicate formula"); err != nil {
		return err
	}

	approvedResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireStatus(approvedResponse, 200, "approve formula"); err != nil {
		return err
	}
	if err := approvedResponse.decode(&formula); err != nil {
		return err
	}
	if formula.State != domain.FormulaApproved || formula.Version != 3 {
		return fmt.Errorf("approved formula has unexpected state or version")
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

	repeated, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve", nil, ptrInt64(3))
	if err != nil {
		return err
	}
	if err := requireCode(repeated, 409, string(domain.CodeFormulaState),
		"approve formula twice"); err != nil {
		return err
	}

	unprofiledResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Unprofiled Formula", ""), nil)
	if err != nil {
		return err
	}
	if err := requireStatus(unprofiledResponse, 201, "create unprofiled formula"); err != nil {
		return err
	}
	var unprofiled formulaView
	if err := unprofiledResponse.decode(&unprofiled); err != nil {
		return err
	}
	stepsResponse, err := api.call("PUT", "/v1/formulas/"+unprofiled.ID+"/steps",
		formulaStepPayload(), ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireStatus(stepsResponse, 200, "set unprofiled steps"); err != nil {
		return err
	}
	unapproved, err := api.call("POST", "/v1/formulas/"+unprofiled.ID+"/approve", nil, ptrInt64(2))
	if err != nil {
		return err
	}
	if err := requireCode(unapproved, 400, string(domain.CodeInvalidInput),
		"approve without a process profile"); err != nil {
		return err
	}
	return nil
}

func ensurePublishedProfile(api *client, name string) (profileView, error) {
	createdResponse, err := api.call("POST", "/v1/profiles",
		createProfilePayload(name, time.Now().UTC().Add(-time.Hour)), nil)
	if err != nil {
		return profileView{}, err
	}
	if err := requireStatus(createdResponse, 201, "create setup profile"); err != nil {
		return profileView{}, err
	}
	var profile profileView
	if err := createdResponse.decode(&profile); err != nil {
		return profileView{}, err
	}
	publishResponse, err := api.call("POST", "/v1/profiles/"+profile.ID+"/publish",
		nil, ptrInt64(profile.Revision))
	if err != nil {
		return profileView{}, err
	}
	if err := requireStatus(publishResponse, 200, "publish setup profile"); err != nil {
		return profileView{}, err
	}
	if err := publishResponse.decode(&profile); err != nil {
		return profileView{}, err
	}
	if profile.State != domain.ProfilePublished {
		return profileView{}, fmt.Errorf("setup profile was not published")
	}
	return profile, nil
}

func ensureApprovedFormula(api *client, name string) (formulaView, error) {
	profile, err := ensurePublishedProfile(api, name+" Profile")
	if err != nil {
		return formulaView{}, err
	}
	return ensureApprovedFormulaWithProfile(api, name, profile.Name)
}

func ensureApprovedFormulaWithProfile(
	api *client,
	name string,
	profileName string,
) (formulaView, error) {
	createdResponse, err := api.call("POST", "/v1/formulas",
		createFormulaPayload(name, profileName), nil)
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
	approveResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(2))
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

func ptrInt64(value int64) *int64 {
	return &value
}
