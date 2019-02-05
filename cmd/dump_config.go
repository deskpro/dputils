package cmd

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	rootCmd.AddCommand(dumpConfigCmd)
}

var dumpConfigCmd = &cobra.Command{
	Use:   "dump_config",
	Short: "Dumps current Deskpro config",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := GetDeskproConfig()

		if err != nil {
			fmt.Println("Could not read config -- maybe it has not been installed?")
			fmt.Println(err)
			os.Exit(1)
		}
		spew.Dump(config)
	},
}