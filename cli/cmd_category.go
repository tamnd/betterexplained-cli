package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) categoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "category <slug>",
		Short: "Articles in a category (e.g. math, programming)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := a.effectiveLimit(20)
			a.progressf("fetching articles in category %q...", args[0])
			articles, err := a.client.Category(cmd.Context(), args[0], n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(articles, len(articles))
		},
	}
}
