package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/go-git/go-git/v6"
)

func runServer(ctx context.Context, opts Options) (*Dispatch, error) {
	token, err := lookupCurrentToken()
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	if token == "" {
		return nil, errors.New("dispatch requires login — run `entire login`")
	}

	now := nowUTC()
	sinceInput := strings.TrimSpace(opts.Since)
	if sinceInput == "" {
		sinceInput = "7d"
	}
	since, err := ParseSinceAtNow(sinceInput, now)
	if err != nil {
		return nil, err
	}
	until, err := ParseUntilAtNow(opts.Until, now)
	if err != nil {
		return nil, err
	}
	normalizedSince, normalizedUntil := NormalizeWindow(since, until)
	if !normalizedSince.Before(normalizedUntil) {
		return nil, errors.New("--since must be before --until")
	}

	repos := append([]string(nil), opts.RepoPaths...)
	orgs := append([]string(nil), opts.Orgs...)
	if len(orgs) == 0 && len(repos) == 0 {
		repoRoot, err := paths.WorktreeRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("not in a git repository: %w", err)
		}
		repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return nil, fmt.Errorf("open repository: %w", err)
		}
		repoFullName, err := resolveRepoFullName(repo)
		if err != nil {
			return nil, err
		}
		repos = []string{repoFullName}
	}

	cloud := NewCloudClient(CloudConfig{BaseURL: api.BaseURL(), Token: token})
	reqBody := CreateDispatchRequest{
		Repos:       repos,
		Orgs:        orgs,
		Since:       normalizedSince.Format(time.RFC3339),
		Until:       normalizedUntil.Format(time.RFC3339),
		Branches:    append([]string(nil), opts.Branches...),
		AllBranches: opts.AllBranches,
		Generate:    true,
		Voice:       resolvedDispatchVoicePreference(opts.Voice),
	}
	response, err := cloud.CreateDispatch(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	dispatch := apiToDispatch(response)
	if strings.TrimSpace(dispatch.GeneratedText) == "" {
		return nil, errDispatchMissingMarkdown
	}
	return dispatch, nil
}

func apiToDispatch(response *CreateDispatchResponse) *Dispatch {
	if response == nil {
		return &Dispatch{}
	}

	repos := make([]RepoGroup, 0, len(response.Repos))
	for _, repo := range response.Repos {
		sections := make([]Section, 0, len(repo.Sections))
		for _, section := range repo.Sections {
			bullets := make([]Bullet, 0, len(section.Bullets))
			for _, bullet := range section.Bullets {
				bullets = append(bullets, Bullet{
					CheckpointID: bullet.CheckpointID,
					Text:         bullet.Text,
					Source:       bullet.Source,
					Branch:       bullet.Branch,
					CreatedAt:    parseAPITime(bullet.CreatedAt),
					Labels:       append([]string(nil), bullet.Labels...),
				})
			}
			sections = append(sections, Section{
				Label:   section.Label,
				Bullets: bullets,
			})
		}
		repos = append(repos, RepoGroup{
			FullName: repo.FullName,
			Sections: sections,
		})
	}

	generatedText := strings.TrimSpace(response.GeneratedMarkdown)
	if generatedText == "" {
		generatedText = strings.TrimSpace(response.GeneratedText)
	}

	return &Dispatch{
		Window: Window{
			NormalizedSince:   parseAPITime(response.Window.NormalizedSince),
			NormalizedUntil:   parseAPITime(response.Window.NormalizedUntil),
			FirstCheckpointAt: parseAPITime(response.Window.FirstCheckpointCreatedAt),
			LastCheckpointAt:  parseAPITime(response.Window.LastCheckpointCreatedAt),
		},
		CoveredRepos:  append([]string(nil), response.CoveredRepos...),
		Repos:         repos,
		GeneratedText: generatedText,
	}
}

func parseAPITime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
