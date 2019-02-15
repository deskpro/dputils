package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(backupCmd)
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup database and/or attachments to the archive",
	Long: `
		Provides various options for backing up a database dump and file attachments from an existing
		source.

		Also it may be used to "publish" the archive for someone you trust.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		dpConfig := Config.ValidateDeskproConfig(cmd)

		if len(dpConfig) > 0 {
			fmt.Println("Config validated")
		}

	},
}