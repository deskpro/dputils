package util

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	phpPath string
	dpPath  string
	config  map[string]string
}

func (config *Config) PhpPath() string {
	return config.phpPath
}

func (config *Config) DpPath() string {
	return config.dpPath
}

func (config *Config) SetPhpPath(phpPath string) *Config {
	config.phpPath = phpPath
	return config
}

func (config *Config) SetDpPath(dpPath string) *Config  {
	config.dpPath = dpPath
	return config
}

func (config *Config) GetDeskproConfig() (map[string]string, error) {

	var err error
	if config.config == nil {
		out, err := exec.Command(config.phpPath, filepath.Join(config.dpPath, "bin", "console"), "dump_config", "--flat-string").Output()
		if err != nil {
			glog.Error("Failed to read config ", err)
			fmt.Println("Command exited with:", out, err)
			return nil, err
		}

		err = json.Unmarshal(out, &config.config)
		if err != nil {
			config.config = nil
		}
	}

	return config.config, err
}

func  (config *Config) ValidateDeskproConfig(cmd *cobra.Command) map[string]string {

	dpConfig, err := config.GetDeskproConfig()

	if err != nil {
		fmt.Println("We failed to read the Deskpro config files. Are they there?")
		fmt.Println("To start fresh, you can install clean config files with this command:")
		fmt.Println("")
		fmt.Println(config.PhpPath(), " ", filepath.Join(config.DpPath(), "bin", "console"), " install:fresh-config")
		fmt.Println("")
		fmt.Println("After config files are inserted, you will need to modify the config.database.php file with your database details.")
		os.Exit(1)
	}

	fmt.Println("==========================================================================================")
	fmt.Println("This Deskpro server")
	fmt.Println("==========================================================================================")

	fmt.Println("We will execute \"" + cmd.Name() + "\" command for this current server. Deskpro is installed here: ")
	fmt.Println("\tDeskpro Path: ", config.dpPath)
	fmt.Println("\tConfig Path: ", filepath.Join(config.dpPath, "config"))

	return dpConfig
}
