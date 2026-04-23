package dispatch

import (
	"context"
	"fmt"
)

type Mode int

const (
	ModeServer Mode = iota
	ModeLocal
)

func (m Mode) String() string {
	switch m {
	case ModeServer:
		return "server"
	case ModeLocal:
		return "local"
	default:
		return fmt.Sprintf("Mode(%d)", m)
	}
}

type Options struct {
	Mode                  Mode
	RepoPaths             []string
	Orgs                  []string
	Since                 string
	Until                 string
	Branches              []string
	AllBranches           bool
	ImplicitCurrentBranch bool
	Voice                 string
}

func Run(ctx context.Context, opts Options) (*Dispatch, error) {
	if opts.Mode == ModeServer {
		return runServer(ctx, opts)
	}
	return runLocal(ctx, opts)
}
