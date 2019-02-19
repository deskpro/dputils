package util

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func CheckDpDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.New("dir is not Deskpro")
	}

	checkDirs := []string{
		"app",
		"bin",
		"www",
	}

	for _, f := range checkDirs {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			return errors.New("dir is not Deskpro")
		}
	}

	return nil
}

func DetectPhpPath() (string, error) {
	if runtime.GOOS == "windows" {
		path, err := exec.LookPath("php.exe")

		if err == nil {
			return path, nil
		}

		check := "C:\\DeskPRO\\PHP\\php.exe"
		if _, err := os.Stat(check); !os.IsNotExist(err) {
			return check, nil
		}
	} else {
		path, err := exec.LookPath("php")

		if err == nil {
			return path, nil
		}
	}

	return "", errors.New("could not find PHP")
}

func DetectDeskproPath() (string, error) {
	dir, _ := os.Getwd()

	if CheckDpDir(dir) == nil {
		return dir, nil
	}

	if runtime.GOOS == "windows" {
		winPath := "C:\\DeskPRO\\DeskPRO"
		if CheckDpDir(winPath) == nil {
			return winPath, nil
		}
	} else {
		vmPath := "/usr/share/nginx/html/deskpro"
		if CheckDpDir(vmPath) == nil {
			return vmPath, nil
		}
	}

	return "", errors.New("CWD is not Deskpro")
}
