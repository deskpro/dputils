package cmd

import (
	"errors"
	"flag"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/golang/glog"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

var cfgFile string
var phpPath string
var dpPath string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dputils",
	Short: "Deskpro tools and utilities for working with helpdesk instances",
	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	flag.Parse()
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dputils.yaml)")
	rootCmd.PersistentFlags().StringVar(&dpPath, "deskpro", "", "Path to Deskpro on the current server")
	rootCmd.PersistentFlags().StringVar(&phpPath,"php", "", "Path to PHP")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if len(phpPath) > 1 {
		glog.Info("Using PHP path specified on CLI: ", phpPath)
	} else {
		phpPath, _ = util.DetectPhpPath()

		if len(phpPath) > 1 {
			glog.Info("Using detected PHP path: ", phpPath)
		} else {
			glog.V(1).Info("Failed to detect PHP path, prompting user")
			fmt.Println("This tool requires PHP to operate correctly. Please enter the path to PHP.")
			prompt := promptui.Prompt{
				Label:    "PHP Path",
				Validate: func(input string) error {
					if _, err := os.Stat(input); os.IsNotExist(err) {
						return errors.New("path to PHP is incorrect")
					}
					_, err := exec.Command(input + " --version").Output()
					if err == nil {
						return errors.New("path to PHP is incorrect")
					}
					return nil
				},
			}
			result, err := prompt.Run()

			if len(result) < 1 || err != nil {
				fmt.Println("Failed to get path to PHP")
				os.Exit(1)
			}

			phpPath = result
			glog.Info("Using PHP path: ", phpPath)
		}
	}

	if len(dpPath) > 1 {
		glog.Info("Using Deskpro path specified on CLI: ", dpPath)
	} else {
		dpPath, _ = util.DetectDeskproPath()

		if len(dpPath) > 1 {
			glog.Info("Using detected Deskpro path: ", dpPath)
		} else {
			fmt.Println("This tool uses Deskpro source files. You can run the tool from within the Deskpro directory, or supply a path here.")
			prompt := promptui.Prompt{
				Label:    "Deskpro Path",
				Validate: func(input string) error {
					if util.CheckDpDir(input) != nil {
						return errors.New("path to Deskpro is incorrect")
					}
					return nil
				},
			}
			result, err := prompt.Run()

			if len(result) < 1 || err != nil {
				fmt.Println("Failed to get path to Deskpro")
				os.Exit(1)
			}

			dpPath = result
			glog.Info("Using Deskpro path: ", dpPath)
		}
	}
}

func GetDeskproConfig() (map[string]string, error) {
	config, err := util.ReadDeskproConfigFile(phpPath, dpPath)
	return config, err
}

func GetPhpPath() string {
	return phpPath
}

func GetDeskproPath() string {
	return dpPath
}