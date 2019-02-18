package cmd

import (
	"errors"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path"
)

var (
	cfgFile string
	phpPath string
	dpPath  string
	Config  util.Config
)


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
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.ErrorLevel)

	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dputils.yaml)")
	rootCmd.PersistentFlags().StringVar(&dpPath, "deskpro", "", "Path to Deskpro on the current server")
	rootCmd.PersistentFlags().StringVar(&phpPath, "php", "", "Path to PHP")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if len(phpPath) > 1 {
		log.Info("Using PHP path specified on CLI: ", phpPath)
	} else {
		phpPath, _ = util.DetectPhpPath()

		if len(phpPath) > 1 {
			log.Info("Using detected PHP path: ", phpPath)
		} else {
			log.Info("Failed to detect PHP path, prompting user")
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
			log.Info("Using PHP path: ", phpPath)
		}
	}

	if len(dpPath) > 1 {
		log.Info("Using Deskpro path specified on CLI: ", dpPath)
	} else {
		dpPath, _ = util.DetectDeskproPath()

		if len(dpPath) > 1 {
			log.Info("Using detected Deskpro path: ", dpPath)
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
			log.Info("Using Deskpro path: ", dpPath)
		}
	}

	log.SetLevel(log.TraceLevel)

	file, err := os.OpenFile(path.Join(dpPath, "var", "logs", "dputils.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(file)
	} else {
		log.Warning("Failed to open dputils.log file. Log output will be directed to stderr instead.")
	}

	Config = util.Config{}
	Config.SetPhpPath(phpPath).SetCfgFile(cfgFile).SetDpPath(dpPath)
}