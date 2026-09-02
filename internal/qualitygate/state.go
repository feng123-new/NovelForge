package qualitygate

import "fmt"

var allowedTransitions = map[TransactionState]map[TransactionState]bool{
	StatePlanned:            {StateDrafting: true, StateHold: true, StateFailed: true},
	StateDrafting:           {StateDraftReady: true, StateHold: true, StateFailed: true},
	StateDraftReady:         {StateLibrarianPending: true, StateHold: true, StateFailed: true},
	StateLibrarianPending:   {StateFactsProposed: true, StateHold: true, StateFailed: true},
	StateFactsProposed:      {StateContinuityPending: true, StateHold: true, StateFailed: true},
	StateContinuityPending:  {StateContinuityPass: true, StateContinuityWarn: true, StateContinuityFail: true, StateHold: true, StateFailed: true},
	StateContinuityPass:     {StateEditorPending: true, StateFinalCandidate: true, StateHold: true},
	StateContinuityWarn:     {StateEditorPending: true, StateRewritePending: true, StateFinalCandidate: true, StateHold: true},
	StateContinuityFail:     {StateRewritePending: true, StateHold: true},
	StateEditorPending:      {StateReviewed: true, StateHold: true, StateFailed: true},
	StateReviewed:           {StateRewritePending: true, StateFinalCandidate: true, StateHold: true},
	StateRewritePending:     {StateDrafting: true, StateFinalCandidate: true, StateHold: true},
	StateFinalCandidate:     {StateTruthCommitPending: true, StateHold: true},
	StateTruthCommitPending: {StateCheckpointPending: true, StateHold: true, StateFailed: true},
	StateCheckpointPending:  {StateCompleted: true, StateHold: true, StateFailed: true},
	StateHold:               {StateDrafting: true, StateLibrarianPending: true, StateFactsProposed: true, StateContinuityPending: true, StateContinuityPass: true, StateContinuityWarn: true, StateContinuityFail: true, StateEditorPending: true, StateReviewed: true, StateFinalCandidate: true, StateTruthCommitPending: true, StateCheckpointPending: true, StateFailed: true},
	StateFailed:             {},
	StateCompleted:          {},
}

func ValidateTransition(from, to TransactionState) error {
	if from == to {
		return nil
	}
	if !allowedTransitions[from][to] {
		return fmt.Errorf("illegal chapter transaction transition %q -> %q", from, to)
	}
	return nil
}
