package cmd

import (
	"errors"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/manifoldco/promptui"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/exec"
)

var cfgFile string
var phpPath string
var dpPath string
var isVerbose bool

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
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dputils.yaml)")
	rootCmd.PersistentFlags().StringVar(&dpPath, "deskpro", "", "Path to Deskpro on the current server")
	rootCmd.PersistentFlags().StringVar(&phpPath,"php", "", "Path to PHP")
	rootCmd.PersistentFlags().BoolVarP(&isVerbose,"verbose", "v", false, "Write logging output to screen")
	_ = viper.BindPFlag("php", rootCmd.PersistentFlags().Lookup("php"))

	if isVerbose {
		log.SetOutput(os.Stdout)
		log.SetLevel(log.TraceLevel)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".dputils" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".dputils")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Debug("Using config file:", viper.ConfigFileUsed())
	}

	if len(phpPath) > 1 {
		log.Info("Using PHP path specified on CLI: ", phpPath)
	} else {
		phpPath, _ = util.DetectPhpPath()

		if len(phpPath) > 1 {
			log.Info("Using detected PHP path: ", phpPath)
		} else {
			log.Debug("Failed to detect PHP path, prompting user")
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

			if err != nil {
				fmt.Println("Failed to get path to PHP")
				os.Exit(1)
			}

			phpPath = result
			log.Info("Using PHP path: ", phpPath)

			viper.Set("php", phpPath)
			_ = rootCmd.PersistentFlags().Set("php", phpPath)
			_ = viper.WriteConfig()
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

			if err != nil {
				fmt.Println("Failed to get path to Deskpro")
				os.Exit(1)
			}

			dpPath = result
			log.Info("Using Deskpro path: ", dpPath)

			viper.Set("deskpro", dpPath)
			_ = rootCmd.PersistentFlags().Set("deskpro", dpPath)
			_ = viper.WriteConfig()
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