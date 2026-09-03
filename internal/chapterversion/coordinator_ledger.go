package chapterversion

import (
	"context"
	"encoding/json"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (c *Coordinator) commitLedger(ctx context.Context, operationID string, final, candidate Version, proposal qualitygate.FactProposal, authority truthstore.Authority) error {
	convert := func(values []qualitygate.FactChange) []narrativeledger.AcceptedChange {
		out := make([]narrativeledger.AcceptedChange, 0, len(values))
		for _, change := range values {
			item := narrativeledger.AcceptedChange{}
			item.Subject = change.Subject
			item.Predicate = change.Predicate
			item.Object = append(json.RawMessage(nil), change.Object...)
			item.SourceChapter = change.SourceChapter
			item.SourceVersion = change.SourceVersion
			item.SourceSHA = change.SourceSHA
			item.Extractor = change.Extractor
			item.Confidence = change.Confidence
			item.Authority = authority
			item.ValidFromChapter = change.ValidFromChapter
			item.ValidToChapter = change.ValidToChapter
			item.KnownFromChapter = change.KnownFromChapter
			item.KnownToChapter = change.KnownToChapter
			item.Reason = change.Reason
			out = append(out, item)
		}
		return out
	}

	input := narrativeledger.AcceptedFinalInput{}
	input.ProjectID = c.Store.projectID
	input.TransactionID = operationID
	input.ProposalID = proposal.ProposalID
	input.CandidateID = final.ID
	input.Chapter = final.Chapter
	input.SourceVersion = candidate.ID
	input.IdempotencyKey = operationID + ":ledger"
	input.ForeshadowUpdates = convert(proposal.ForeshadowUpdates)
	input.Secrets = convert(proposal.Secrets)
	if _, err := c.Ledger.CommitAcceptedFinal(ctx, input); err != nil {
		return newError(CodeStorage, "Narrative Ledger commit failed", true, err)
	}
	if authority == truthstore.AuthorityHumanFinal {
		promotion := narrativeledger.HumanAuthorityPromotion{}
		promotion.ProjectID = c.Store.projectID
		promotion.TransactionID = operationID
		promotion.CandidateID = final.ID
		promotion.Chapter = final.Chapter
		promotion.SourceVersion = candidate.ID
		promotion.IdempotencyKey = operationID + ":human-authority"
		if _, err := c.Ledger.PromoteAcceptedFinalAuthority(ctx, promotion); err != nil {
			return newError(CodeStorage, "Narrative Ledger Human Final authority promotion failed", true, err)
		}
	}
	return nil
}
