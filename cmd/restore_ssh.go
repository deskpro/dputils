package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	restoreCmd.Flags().String(
		"ssh",
		"",
		`
			URI describing SSH credentials. If user is specified without a password, then you will be prompted for it.
			If neither user or password is specified, we expect the current SSH config to have a value for the host.
			If you want to use a key instead of a password, specify the i=/path/to/key param

			Examples:
				deskpro:pass@10.1.1.3/path/to/deskpro
				myuser@10.1.1.3/path/to/deskpro?i=~/.ssh/mykey
				10.1.1.3/path/to/deskpro
		`,
	)

	rootCmd.AddCommand(restoreSshCmd)
}

var restoreSshCmd = &cobra.Command{
	Use:   "restore_ssh",
	Short: "Restore Deskpro to the current server using data from some other server.",
	Long: `
		Provides various options for downloading a database dump and file attachments from an existing
		source, and then imports it into the current server.

		Any option that accepts a remote URI supports the following protocols: http, https, ftp, sftp.
	`,
	Run: func(cmd *cobra.Command, args []string) {

	},
}
