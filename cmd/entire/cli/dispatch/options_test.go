package dispatch

import "testing"

func TestResolveOptions_NormalizesScopeValues(t *testing.T) {
	t.Parallel()

	opts, err := ResolveOptions(
		false,
		"7d",
		"",
		false,
		[]string{" entireio/cli ", "", "entireio/cli"},
		[]string(nil),
		"",
		func() (string, error) { return "main", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.RepoPaths) != 1 || opts.RepoPaths[0] != "entireio/cli" {
		t.Fatalf("unexpected normalized repo paths: %v", opts.RepoPaths)
	}
}
