package workflowcheck

import (
	"fmt"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

// profileBinding verifies that each run keeps the exact profile version it
// adopted at start: publishing a newer version or withdrawing the bound one
// must not rewrite a run that is already fermenting.
func profileBinding(api *client) error {
	profile, err := ensurePublishedProfile(api, "Binding Ale")
	if err != nil {
		return err
	}
	if profile.Version != 1 {
		return fmt.Errorf("first profile version should be 1, got %d", profile.Version)
	}

	duplicate, err := api.call("POST", "/v1/profiles",
		createProfilePayload("  binding   ale  ", time.Now().UTC().Add(-time.Hour)), nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeProfileNameConflict),
		"create duplicate profile family"); err != nil {
		return err
	}

	formula, err := ensureApprovedFormulaWithProfile(api, "Binding Formula", profile.Name)
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

	startedA, err := createStartedRun(api, formula.ID, vesselA.ID, "RUN-PROFILE-A")
	if err != nil {
		return err
	}
	if err := requireBinding(startedA.Run, 1); err != nil {
		return err
	}

	// Publish a stricter second version while run A is fermenting.
	reviseResponse, err := api.call("POST", "/v1/profiles/"+profile.ID+"/revise",
		map[string]any{"effective_at": time.Now().UTC().Add(-time.Minute)},
		ptrInt64(profile.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(reviseResponse, 201, "revise profile"); err != nil {
		return err
	}
	var draft profileView
	if err := reviseResponse.decode(&draft); err != nil {
		return err
	}
	if draft.Version != 2 || draft.State != domain.ProfileDraft {
		return fmt.Errorf("revised profile has unexpected version or state")
	}

	updatedResponse, err := api.call("PUT", "/v1/profiles/"+draft.ID,
		map[string]any{
			"min_fermentation_temp_c":       10.0,
			"max_fermentation_temp_c":       25.0,
			"min_ferment_days":              0,
			"max_ferment_days":              120,
			"min_observations_to_condition": 4,
			"min_observations_to_complete":  4,
			"effective_at":                  time.Now().UTC().Add(-time.Minute),
		}, ptrInt64(draft.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(updatedResponse, 200, "tighten draft profile"); err != nil {
		return err
	}
	if err := updatedResponse.decode(&draft); err != nil {
		return err
	}

	publishResponse, err := api.call("POST", "/v1/profiles/"+draft.ID+"/publish",
		nil, ptrInt64(draft.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(publishResponse, 200, "publish revised profile"); err != nil {
		return err
	}
	var publishedV2 profileView
	if err := publishResponse.decode(&publishedV2); err != nil {
		return err
	}

	republish, err := api.call("POST", "/v1/profiles/"+draft.ID+"/publish",
		nil, ptrInt64(publishedV2.Revision))
	if err != nil {
		return err
	}
	if err := requireCode(republish, 409, string(domain.CodeProfileState),
		"publish the same draft twice"); err != nil {
		return err
	}

	// Withdraw version 1 while run A still ferments; run A keeps v1.
	withdrawResponse, err := api.call("POST", "/v1/profiles/"+profile.ID+"/withdraw",
		nil, ptrInt64(profile.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(withdrawResponse, 200, "withdraw superseded profile"); err != nil {
		return err
	}

	base := time.Now().UTC().Add(10 * time.Second)
	runA, err := appendCheckObservations(api, startedA.Run.ID, startedA.Run.Version, base,
		1, []float64{1.050, 1.030, 1.015}, []float64{20.0, 20.0, 20.0})
	if err != nil {
		return err
	}
	conditioningA, err := api.call("POST", "/v1/runs/"+startedA.Run.ID+"/conditioning",
		nil, ptrInt64(runA.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(conditioningA, 200, "condition run A"); err != nil {
		return err
	}
	if err := conditioningA.decode(&runA); err != nil {
		return err
	}
	completeA, err := api.call("POST", "/v1/runs/"+startedA.Run.ID+"/complete",
		nil, ptrInt64(runA.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(completeA, 200,
		"complete run A under its bound version"); err != nil {
		return err
	}
	var completedA pairView
	if err := completeA.decode(&completedA); err != nil {
		return err
	}
	if err := requireBinding(completedA.Run, 1); err != nil {
		return err
	}

	// A run started after version 2 took effect binds version 2 instead.
	startedB, err := createStartedRun(api, formula.ID, vesselB.ID, "RUN-PROFILE-B")
	if err != nil {
		return err
	}
	if err := requireBinding(startedB.Run, 2); err != nil {
		return err
	}

	// Version 2 demands four observations before conditioning; three are not
	// enough. Observations freeze once conditioning begins, so the threshold
	// must bite while the run is still fermenting.
	baseB := time.Now().UTC().Add(10 * time.Second)
	runB, err := appendCheckObservations(api, startedB.Run.ID, startedB.Run.Version, baseB,
		1, []float64{1.050, 1.032, 1.020}, []float64{20.0, 20.0, 20.0})
	if err != nil {
		return err
	}
	earlyConditioning, err := api.call("POST", "/v1/runs/"+startedB.Run.ID+"/conditioning",
		nil, ptrInt64(runB.Version))
	if err != nil {
		return err
	}
	if err := requireCode(earlyConditioning, 409, string(domain.CodeRunState),
		"condition run B below the v2 observation threshold"); err != nil {
		return err
	}

	runB, err = appendCheckObservations(api, startedB.Run.ID, runB.Version, baseB,
		4, []float64{1.015}, []float64{20.0})
	if err != nil {
		return err
	}
	conditioningB, err := api.call("POST", "/v1/runs/"+startedB.Run.ID+"/conditioning",
		nil, ptrInt64(runB.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(conditioningB, 200, "condition run B"); err != nil {
		return err
	}
	if err := conditioningB.decode(&runB); err != nil {
		return err
	}
	completeB, err := api.call("POST", "/v1/runs/"+startedB.Run.ID+"/complete",
		nil, ptrInt64(runB.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(completeB, 200, "complete run B under v2"); err != nil {
		return err
	}

	// A third run shows version 2's narrowed temperature band (10-25 Celsius)
	// rejecting completion when the final observation is out of band.
	vesselC, err := createCheckVessel(api, "VSL-303", 300)
	if err != nil {
		return err
	}
	startedC, err := createStartedRun(api, formula.ID, vesselC.ID, "RUN-PROFILE-C")
	if err != nil {
		return err
	}
	if err := requireBinding(startedC.Run, 2); err != nil {
		return err
	}
	baseC := time.Now().UTC().Add(10 * time.Second)
	runC, err := appendCheckObservations(api, startedC.Run.ID, startedC.Run.Version, baseC,
		1, []float64{1.050, 1.032, 1.020, 1.015}, []float64{20.0, 20.0, 20.0, 28.0})
	if err != nil {
		return err
	}
	conditioningC, err := api.call("POST", "/v1/runs/"+startedC.Run.ID+"/conditioning",
		nil, ptrInt64(runC.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(conditioningC, 200, "condition run C"); err != nil {
		return err
	}
	if err := conditioningC.decode(&runC); err != nil {
		return err
	}
	hotComplete, err := api.call("POST", "/v1/runs/"+startedC.Run.ID+"/complete",
		nil, ptrInt64(runC.Version))
	if err != nil {
		return err
	}
	if err := requireCode(hotComplete, 409, string(domain.CodeProfileViolation),
		"complete run C outside the v2 temperature band"); err != nil {
		return err
	}

	// Withdrawing version 2 afterwards must not rewrite the completed run.
	staleWithdraw, err := api.call("POST", "/v1/profiles/"+draft.ID+"/withdraw",
		nil, ptrInt64(1))
	if err != nil {
		return err
	}
	if err := requireCode(staleWithdraw, 412, string(domain.CodeRevisionConflict),
		"withdraw with a stale revision"); err != nil {
		return err
	}
	withdrawV2, err := api.call("POST", "/v1/profiles/"+draft.ID+"/withdraw",
		nil, ptrInt64(publishedV2.Revision))
	if err != nil {
		return err
	}
	if err := requireStatus(withdrawV2, 200, "withdraw version 2"); err != nil {
		return err
	}

	readB, err := api.call("GET", "/v1/runs/"+startedB.Run.ID, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(readB, 200, "read completed run B"); err != nil {
		return err
	}
	var finalB runView
	if err := readB.decode(&finalB); err != nil {
		return err
	}
	if err := requireBinding(finalB, 2); err != nil {
		return err
	}
	return nil
}

func requireBinding(run runView, version int) error {
	if run.ProfileBinding == nil {
		return fmt.Errorf("run carries no profile binding")
	}
	if run.ProfileBinding.ProfileVersion != version {
		return fmt.Errorf("run bound profile version %d, expected %d",
			run.ProfileBinding.ProfileVersion, version)
	}
	if run.ProfileBinding.Reason != domain.BindingReasonRunStart {
		return fmt.Errorf("run binding reason %q is unexpected", run.ProfileBinding.Reason)
	}
	return nil
}

func appendCheckObservations(
	api *client,
	runID string,
	version int64,
	base time.Time,
	startSequence int,
	gravities []float64,
	temperatures []float64,
) (runView, error) {
	var run runView
	for index, gravity := range gravities {
		sequence := startSequence + index
		response, err := api.call("POST", "/v1/runs/"+runID+"/observations",
			map[string]any{
				"sequence":      sequence,
				"gravity":       gravity,
				"temperature_c": temperatures[index],
				"observed_at":   base.Add(time.Duration(sequence) * 10 * time.Second),
			}, ptrInt64(version))
		if err != nil {
			return runView{}, err
		}
		if err := requireStatus(response, 200, "append binding observation"); err != nil {
			return runView{}, err
		}
		if err := response.decode(&run); err != nil {
			return runView{}, err
		}
		version = run.Version
	}
	return run, nil
}
