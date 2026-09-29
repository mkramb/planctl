package main

import (
	"github.com/mkramb/planctl/internal/plan"
	"github.com/spf13/cobra"
)

type reviewFlags struct {
	reviewers []string
	title     string
	base      string
}

func addReviewFlags(cmd *cobra.Command, f *reviewFlags) {
	cmd.Flags().StringArrayVar(&f.reviewers, "reviewer", nil, "Request a reviewer (repeatable); every reviewer must approve")
	cmd.Flags().StringVar(&f.title, "title", "", "Override the review title")
	cmd.Flags().StringVar(&f.base, "base", "", "Override the base branch")
}

func (f reviewFlags) request(deps Dependencies, opts *options, files []string) plan.PublishRequest {
	return plan.PublishRequest{
		Dir: deps.Dir, ConfigPath: opts.config, Files: files,
		Title: f.title, Base: f.base, Reviewers: f.reviewers,
	}
}
