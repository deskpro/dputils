package cmd

import (
	"archive/zip"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	backupCmd.Flags().StringP(
		"target",
		"t",
		"",
		`
				Where to save the file to. A directory name to put a
                deskpro-backup.DATE.zip file, or a filename.zip to specify
                the exact file.

                Use the special target "public" to save the backup to Deskpro's
                public directory. This will give you a long random URL that
                you can use to expose the download to the internet. This
                can be useful if you want an wasy way to get this backup to
                another person or server. Just remember to delete the file
                after it's been deleted!

  				Provide "database" or "attachments" to backup just that thing. If not specified, both are backed up.
		`,
	)

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

		var targetName string

		target, _ := cmd.Flags().GetString("target")

		if target == "public" {
			targetName = filepath.Join(Config.DpPath(), "www", "assets", "deskpro_backup" + time.Now().Format("Y_m_d_h_i_s") + ".zip")
		} else {
			targetName = target
		}

		newZipFile, err := os.Create(targetName)

		if err != nil {
			fmt.Println("Could not create backup archive:")
			fmt.Println(err)
		}

		defer newZipFile.Close()
		zipFile := zip.NewWriter(newZipFile)
		defer zipFile.Close()
		addDumpToZipFile(dpConfig, "", zipFile)
		addDumpToZipFile(dpConfig, "audit", zipFile)
		addDumpToZipFile(dpConfig, "voice", zipFile)
		addDumpToZipFile(dpConfig, "system", zipFile)
		if len(dpConfig) > 0 {
			fmt.Println(targetName)
		}
	},
}

func addDumpToZipFile(dpConfig map[string]string, dbType string, zipFile *zip.Writer) {
	var prefix string
	if dbType == "" {
		prefix = "database"
	} else {
		prefix = "database_advanced." + dbType
	}
	databaseUrl := util.GetMysqlUrlFromConfig(dpConfig, prefix)
	if databaseUrl.User.Username() == "" {
		return
	}
	databaseMysqlPass, _ := databaseUrl.User.Password()
	databaseMysqlPort := databaseUrl.Port()
	if len(databaseMysqlPort) < 1 {
		databaseMysqlPort = "3306"
	}
	remoteArgs := []string{
		"--opt -Q --hex-blob --lock-tables=false --single-transaction",
		"-h", databaseUrl.Host,
		"--port", databaseMysqlPort,
		"-u", databaseUrl.User.Username(),
		"-C",
	}
	if databaseMysqlPass != "" {
		remoteArgs = append(remoteArgs, "--password=remoteMysqlPass")
	}
	remoteArgs = append(remoteArgs, strings.TrimLeft(databaseUrl.Path, "/"))

	mysqlDumpBin := dpConfig["paths.mysqldump_path"]

	reader, writer := io.Pipe()
	dumpCmd := exec.Command(
		mysqlDumpBin,
		remoteArgs...
	)

	dumpCmd.Stdout = writer
	zipWriter, _ := zipFile.Create(prefix + ".sql")
	go func() {
		defer reader.Close()
		if _, err := io.Copy(zipWriter, reader); err != nil {
			fmt.Println(err)
		}
	}()

	if err := dumpCmd.Run(); err != nil {
		fmt.Println("Failed to write a dump file to zip archive")
		fmt.Println(err)
	}
}