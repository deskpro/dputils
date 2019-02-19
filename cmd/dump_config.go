package cmd

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(dumpConfigCmd)
}

var dumpConfigCmd = &cobra.Command{
	Use:   "dump_config",
	Short: "Dumps current Deskpro config",
	Run: func(cmd *cobra.Command, args []string) {
		dpConfig := Config.ValidateDeskproConfig(cmd)
		spew.Dump(dpConfig)
	},
}