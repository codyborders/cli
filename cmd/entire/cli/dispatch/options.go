package dispatch

import (
	"errors"
	"strings"
)

func ResolveOptions(
	flagLocal bool,
	flagSince string,
	flagUntil string,
	flagAllBranches bool,
	flagRepos []string,
	flagOrgs []string,
	flagVoice string,
	currentBranch func() (string, error),
) (Options, error) {
	flagRepos = normalizeScopeValues(flagRepos)
	flagOrgs = normalizeScopeValues(flagOrgs)
	if len(flagOrgs) > 0 && len(flagRepos) > 0 {
		return Options{}, errors.New("--org and --repos are mutually exclusive")
	}
	if flagLocal && len(flagRepos) > 0 {
		return Options{}, errors.New("--repos cannot be used with --local")
	}
	if flagLocal && len(flagOrgs) > 0 {
		return Options{}, errors.New("--org cannot be used with --local")
	}

	mode := ModeServer
	if flagLocal {
		mode = ModeLocal
	}

	var branches []string
	allBranches := flagAllBranches
	implicitCurrentBranch := false
	switch {
	case allBranches:
	case len(flagRepos) > 0, len(flagOrgs) > 0:
		branches = nil
	default:
		currentBranchName, err := currentBranch()
		if err != nil {
			return Options{}, err
		}
		branches = []string{currentBranchName}
		implicitCurrentBranch = true
	}

	return Options{
		Mode:                  mode,
		RepoPaths:             flagRepos,
		Orgs:                  flagOrgs,
		Since:                 flagSince,
		Until:                 flagUntil,
		Branches:              branches,
		AllBranches:           allBranches,
		ImplicitCurrentBranch: implicitCurrentBranch,
		Voice:                 flagVoice,
	}, nil
}

func normalizeScopeValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
