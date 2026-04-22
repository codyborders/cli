package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	dispatchpkg "github.com/entireio/cli/cmd/entire/cli/dispatch"
)

func TestNewDispatchWizardState_Defaults(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	if state.modeChoice != dispatchWizardModeLocal {
		t.Fatalf("expected local mode default, got %q", state.modeChoice)
	}
	if state.scopeType != dispatchWizardScopeCurrentRepo {
		t.Fatalf("expected current repo scope default, got %q", state.scopeType)
	}
	if state.timeWindowPreset != "7d" {
		t.Fatalf("expected 7d default, got %q", state.timeWindowPreset)
	}
	if state.branchMode != dispatchWizardBranchCurrent {
		t.Fatalf("expected current-branch mode default, got %q", state.branchMode)
	}
	if state.voicePreset != testDispatchVoicePresetNeutral {
		t.Fatalf("expected neutral voice preset default, got %q", state.voicePreset)
	}
	if state.voiceCustom != "" {
		t.Fatalf("expected empty custom voice default, got %q", state.voiceCustom)
	}
	if !state.confirmRun {
		t.Fatal("expected run confirmation to default to true")
	}
}

func TestDispatchWizardState_ResolveOrgDefaultsToDefaultBranches(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.modeChoice = dispatchWizardModeServer
	state.scopeType = dispatchWizardScopeOrganization
	state.selectedOrgs = []string{"entireio"}

	opts, err := state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(opts.Orgs, ","); got != "entireio" {
		t.Fatalf("expected single org selection to resolve to org scope, got %q", got)
	}
	if len(opts.RepoPaths) != 0 {
		t.Fatalf("expected single org selection not to expand to repo paths, got %v", opts.RepoPaths)
	}
	if opts.AllBranches {
		t.Fatal("did not expect org scope to default to all branches")
	}
	if opts.Branches != nil {
		t.Fatalf("expected nil branches, got %v", opts.Branches)
	}

	state.selectedOrgs = []string{"entireio", "entirehq"}
	opts, err = state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(opts.Orgs, ","); got != "entireio,entirehq" {
		t.Fatalf("expected multi-org selection to resolve to org scope, got %q", got)
	}
	if len(opts.RepoPaths) != 0 {
		t.Fatalf("expected multi-org selection not to expand to matching repos, got %v", opts.RepoPaths)
	}
}

func TestDispatchWizardState_ResolveAllBranches(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.branchMode = dispatchWizardBranchAll

	opts, err := state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !opts.AllBranches {
		t.Fatal("expected all branches")
	}
}

func TestDispatchWizardState_LocalBranchModes(t *testing.T) {
	t.Parallel()

	values := optionValues(newDispatchWizardState().branchModeOptions())
	if got := strings.Join(values, ","); got != dispatchWizardBranchCurrent+","+dispatchWizardBranchAll {
		t.Fatalf("unexpected local branch modes: %v", values)
	}
}

func TestDispatchWizardState_ServerBranchModes(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.modeChoice = dispatchWizardModeServer

	values := optionValues(state.branchModeOptions())
	if got := strings.Join(values, ","); got != dispatchWizardBranchDefault+","+dispatchWizardBranchAll {
		t.Fatalf("unexpected server branch modes: %v", values)
	}
}

func TestDispatchWizardState_ScopeOptionsAdaptByMode(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	localScopeValues := optionValues(state.scopeOptions())
	if got := strings.Join(localScopeValues, ","); got != dispatchWizardScopeCurrentRepo {
		t.Fatalf("unexpected local scope options: %v", localScopeValues)
	}

	state.modeChoice = dispatchWizardModeServer
	serverScopeValues := optionValues(state.scopeOptions())
	if got := strings.Join(serverScopeValues, ","); got != dispatchWizardScopeSelectedRepos+","+dispatchWizardScopeOrganization {
		t.Fatalf("unexpected server scope options: %v", serverScopeValues)
	}
}

func TestBuildDispatchRepoOptions_UsesFullSlugLabels(t *testing.T) {
	t.Parallel()

	options := buildDispatchRepoOptions([]string{"entireio/entire.io", "entireio/cli"})
	if got := strings.Join(optionKeys(options), ","); got != "entireio/cli,entireio/entire.io" {
		t.Fatalf("expected repo options to use org/repo labels sorted, got %q", got)
	}
}

func TestDispatchWizardState_ServerModeKeepsSelectedReposScope(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.modeChoice = dispatchWizardModeServer
	state.scopeType = dispatchWizardScopeSelectedRepos
	state.selectedRepos = []string{"entireio/cli"}

	if got := state.effectiveScopeType(); got != dispatchWizardScopeSelectedRepos {
		t.Fatalf("expected server mode to keep selected repos scope, got %q", got)
	}

	opts, err := state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatalf("expected server mode to resolve selected repos, got %v", err)
	}
	if got := strings.Join(opts.RepoPaths, ","); got != "entireio/cli" {
		t.Fatalf("expected selected repo path to propagate, got %q", got)
	}
}

func TestDispatchWizardState_ShowsRepoPickerOnlyForSelectedRepos(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	if state.showRepoPicker() {
		t.Fatal("did not expect repo picker in local mode")
	}

	state.modeChoice = dispatchWizardModeServer
	state.scopeType = dispatchWizardScopeSelectedRepos
	if !state.showRepoPicker() {
		t.Fatal("expected repo picker for selected repos scope in server mode")
	}
}

func TestDispatchWizardState_ShowsOrganizationPickerOnlyForOrgScope(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	if state.showOrganizationPicker() {
		t.Fatal("did not expect organization picker in local mode")
	}

	state.modeChoice = dispatchWizardModeServer
	if state.showOrganizationPicker() {
		t.Fatal("did not expect organization picker for current repo server scope")
	}

	state.scopeType = dispatchWizardScopeOrganization
	if !state.showOrganizationPicker() {
		t.Fatal("expected organization picker for organization scope")
	}
}

func TestDispatchWizardState_ResolveVoiceInput(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.voicePreset = testDispatchVoicePresetMarvin
	opts, err := state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if opts.Voice != testDispatchVoicePresetMarvin {
		t.Fatalf("expected marvin voice, got %q", opts.Voice)
	}

	state.voicePreset = testDispatchVoicePresetNeutral
	opts, err = state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if opts.Voice != testDispatchVoicePresetNeutral {
		t.Fatalf("expected neutral voice, got %q", opts.Voice)
	}
	state.voicePreset = testDispatchVoicePresetCustom
	state.voiceCustom = "dry, skeptical release note narrator"
	opts, err = state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if opts.Voice != "dry, skeptical release note narrator" {
		t.Fatalf("expected custom voice, got %q", opts.Voice)
	}
}

func TestDispatchWizardState_ResolveEmptyVoiceDefaultsToNeutral(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	state.voicePreset = testDispatchVoicePresetCustom
	state.voiceCustom = "   "

	opts, err := state.resolve(func() (string, error) { return testDispatchPreviewBranch, nil })
	if err != nil {
		t.Fatal(err)
	}
	if opts.Voice != testDispatchVoicePresetNeutral {
		t.Fatalf("expected neutral voice fallback, got %q", opts.Voice)
	}
}

func TestDispatchWizardState_ShowsCustomVoiceInputOnlyForCustomPreset(t *testing.T) {
	t.Parallel()

	state := newDispatchWizardState()
	if state.showCustomVoiceInput() {
		t.Fatal("did not expect custom voice input for default preset")
	}

	state.voicePreset = testDispatchVoicePresetCustom
	if !state.showCustomVoiceInput() {
		t.Fatal("expected custom voice input for custom preset")
	}
}

func TestBuildDispatchWizardSummary(t *testing.T) {
	t.Parallel()

	summary := buildDispatchWizardSummary(dispatchpkg.Options{
		Mode:        dispatchpkg.ModeLocal,
		RepoPaths:   []string{"/tmp/repo-a", "/tmp/repo-b"},
		Branches:    nil,
		AllBranches: false,
	}, "")
	if !strings.Contains(summary, "Mode: local") {
		t.Fatalf("expected local mode in summary, got %q", summary)
	}
	if !strings.Contains(summary, "Scope: repos:/tmp/repo-a, /tmp/repo-b") {
		t.Fatalf("expected repo scope in summary, got %q", summary)
	}
	if !strings.Contains(summary, "Branches: current branch") {
		t.Fatalf("expected branches in summary, got %q", summary)
	}

	summary = buildDispatchWizardSummary(dispatchpkg.Options{
		Mode:        dispatchpkg.ModeServer,
		RepoPaths:   []string{"entireio/cli"},
		AllBranches: false,
	}, "")
	if !strings.Contains(summary, "Mode: cloud") {
		t.Fatalf("expected cloud mode in summary, got %q", summary)
	}
}

func TestBuildDispatchCommand(t *testing.T) {
	t.Parallel()

	command := buildDispatchCommand(dispatchpkg.Options{
		Mode:        dispatchpkg.ModeServer,
		Since:       "7d",
		Branches:    nil,
		Voice:       testDispatchVoicePresetMarvin,
		RepoPaths:   []string{"entireio/cli"},
		AllBranches: false,
	})
	if !strings.Contains(command, "entire dispatch") {
		t.Fatalf("expected base command, got %q", command)
	}
	if !strings.Contains(command, "--voice marvin") {
		t.Fatalf("expected preset voice flag, got %q", command)
	}
	if !strings.Contains(command, "--repos entireio/cli") {
		t.Fatalf("expected server repos flag, got %q", command)
	}
	if strings.Contains(command, "--local") {
		t.Fatalf("did not expect local flag for server mode, got %q", command)
	}
	if strings.Contains(command, "--all-branches") {
		t.Fatalf("did not expect all-branches flag when AllBranches is false, got %q", command)
	}
}

func TestBuildDispatchCommand_AllBranches(t *testing.T) {
	t.Parallel()

	command := buildDispatchCommand(dispatchpkg.Options{
		Mode:        dispatchpkg.ModeServer,
		Since:       "7d",
		Voice:       testDispatchVoicePresetMarvin,
		RepoPaths:   []string{"entireio/cli"},
		AllBranches: true,
	})
	if !strings.Contains(command, "--all-branches") {
		t.Fatalf("expected all-branches flag, got %q", command)
	}
}

func TestBuildDispatchOrgOptions_SortsNames(t *testing.T) {
	t.Parallel()

	options := buildDispatchOrgOptions([]string{"beta", "alpha"})
	if got := strings.Join(optionValues(options), ","); got != "alpha,beta" {
		t.Fatalf("unexpected org options: %v", optionValues(options))
	}
}

func TestBuildDispatchRepoOptions_DedupesAndSorts(t *testing.T) {
	t.Parallel()

	options := buildDispatchRepoOptions([]string{"entireio/entire.io", "entireio/cli", "entireio/cli"})
	if got := strings.Join(optionValues(options), ","); got != "entireio/cli,entireio/entire.io" {
		t.Fatalf("unexpected repo options: %v", optionValues(options))
	}
}

func optionValues(options []huh.Option[string]) []string {
	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, option.Value)
	}
	return values
}

func optionKeys(options []huh.Option[string]) []string {
	keys := make([]string, 0, len(options))
	for _, option := range options {
		keys = append(keys, option.Key)
	}
	return keys
}
