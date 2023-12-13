package cmd

import (
	"github.com/Azure/draft/test/e2e/logger"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Runs e2e tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		lgr := logger.FromContext(ctx)
		lgr.Info("Running e2e tests")

		if len(args) == 0 {
			lgr.Info("No tests specified, running all tests")
			return runAllTests(ctx)
		}

		suitesToRun := args
		for _, suite := range suitesToRun {
			lgr.Infof("Running test suite %s", suite)
			if err := runTestSuite(ctx, suite); err != nil {
				return err
			}
		}

		return nil
	},
}
