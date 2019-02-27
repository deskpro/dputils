package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"testing"
)

func Test_checkFullBackup(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringP(
		"full-backup",
		"k",
		"",
		"",
	)
	_ = cmd.Flags().Set("full-backup", filepath.Join("..", "test_mocks", "backup.zip"))
	fullBackup, backup := checkFullBackup(cmd, filepath.Join("..", "test_mocks", "tmp"))

	if fullBackup != true {
		t.Error("Backup checking failed!")
	}

	// clean after
	_ = os.RemoveAll(backup)
}

func Test_getFullBackupDump(t *testing.T) {
	dumpFile := getFullBackupDump(filepath.Join("..", "test_mocks"), "database")

	if dumpFile == "" {
		t.Error("Backup checking failed!")
	}

	dumpFile = getFullBackupDump(filepath.Join("..", "test_mocks"), "database_compressed")

	if dumpFile == "" {
		t.Error("Backup checking failed!")
	}
}