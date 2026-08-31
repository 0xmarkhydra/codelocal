package editing

import "errors"

var ErrReconcileNotApplicable = errors.New("reconciliation result has no agent mutation to apply")

// PatchFileFromReconcile bridges pure reconciliation into PatchSet V2. The
// latest authoritative hash becomes the optimistic write guard, so another user
// edit between reconciliation and apply turns into STALE instead of overwrite.
func PatchFileFromReconcile(result ReconcileResult) (PatchFile, error) {
	if !result.CanApply || result.ResultHash == "" || result.LatestHash == "" {
		return PatchFile{}, ErrReconcileNotApplicable
	}
	content := string(result.Content)
	return PatchFile{
		Path:         result.Path,
		ExpectedHash: result.LatestHash,
		Content:      &content,
	}, nil
}
