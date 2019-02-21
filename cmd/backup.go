package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/spf13/cobra"
	"gopkg.in/cheggaaa/pb.v2"
	"io"
	"io/ioutil"
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
                after it's been downloaded!

  				Provide "database" or "attachments" to backup just that thing. If not specified, both are backed up.
		`,
	)

	backupCmd.Flags().StringP(
		"backup",
		"b",
		"",
		`
				Provide "database" or "attachments" to backup just
                that thing. If not specified, both are backed up.
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
		target, _ = filepath.Abs(target)
		fileName := "deskpro-backup." + time.Now().Format("2006-01-02_15:04:05") + ".zip"
		if target == "public" {
			targetName = filepath.Join(Config.DpPath(), "www", "assets", fileName)
		} else {
			targetName = target
			info, err := os.Stat(target)
			ext := filepath.Ext(target)
			if err != nil && os.IsNotExist(err) && ext == "" {
				fmt.Println("Can't find specified dir")
			} else if err != nil && os.IsNotExist(err) && ext != "" {
				targetName = target
			} else if info.IsDir() {
				targetName = filepath.Join(targetName, fileName)
			} else if err != nil {
				fmt.Println("Can't find target path, please check your --target option carefully")
				fmt.Println(err)
			}

		}

		what, _ := cmd.Flags().GetString("backup")
		if what != "attachments" && what != "database" && what != "" {
			fmt.Println("Wrong --backup options, you may specify either \"attachments\" or \"database\" or omit the option to backup both")
			os.Exit(1)
		}

		fmt.Println("Backing up to " + targetName)

		zipFile, err := os.Create(targetName)
		if err != nil {
			fmt.Println("Could not create backup archive:")
			fmt.Println(err)
		}
		defer zipFile.Close()
		zipFileWriter := zip.NewWriter(zipFile)
		defer zipFileWriter.Close()
		if what == "database" || what == "" {
			addDumpToTheZipFile(dpConfig, "", zipFileWriter)
			addDumpToTheZipFile(dpConfig, "audit", zipFileWriter)
			addDumpToTheZipFile(dpConfig, "voice", zipFileWriter)
			addDumpToTheZipFile(dpConfig, "system", zipFileWriter)
		}
		if what == "attachments" || what == "" {
			addAttachmentsToTheZipFile(dpConfig, Config.DpPath(), zipFileWriter)
		}



		if target == "public" {
			targetName = "http://your-deskpro-url/assets/" + fileName
		}

		fmt.Println("Your backup is available at " + targetName)
	},
}

func addAttachmentsToTheZipFile(dpConfig map[string]string, dpPath string, zipFile *zip.Writer) {
	fmt.Println("Writing attachments")
	var attachUri string
	if val, ok := dpConfig["paths.dp_paths.attachments"]; ok {
		attachUri = val
	} else {
		attachUri = filepath.Join(dpPath, "attachments")
	}

	addFilesToTheZip(zipFile, attachUri, "attachments")
	fmt.Println("\t Done writing attachments")
}

func addFilesToTheZip(zipFile *zip.Writer, uri string, zipPath string) {
	files, err := ioutil.ReadDir(uri)
	if err != nil {
		fmt.Println(err)
	}

	var size int64
	bar := pb.ProgressBarTemplate(`{{ blue "Processing: ` + uri + `" }} {{bar . | green}} {{speed . | blue }}`).Start(len(files))
	for _, file := range files {
		// we don't want to backup import temporary files
		if file.Name() == "import" {
			bar.Increment()
			continue
		}
		if !file.IsDir() {
			size += file.Size()
			dat, err := ioutil.ReadFile(filepath.Join(uri, file.Name()))
			if err != nil {
				fmt.Println(err)
			}

			f, err := zipFile.Create(filepath.Join(zipPath, file.Name()))
			if err != nil {
				fmt.Println(err)
			}
			_, err = f.Write(dat)
			if err != nil {
				fmt.Println(err)
			}
			if size > 10 * 1024 * 1024 {
				if err := zipFile.Flush(); err != nil {
					fmt.Println("Can't flush data")
					os.Exit(1)
				}
				size = 0
			}
			bar.Increment()
		} else if file.IsDir() {
			newBase := filepath.Join(uri, file.Name(), "")
			bar.Increment()
			addFilesToTheZip(zipFile, newBase, filepath.Join(zipPath, file.Name(), ""))
		}
	}
	bar.Finish()
}

func addDumpToTheZipFile(dpConfig map[string]string, dbType string, zipFile *zip.Writer) {

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

	dbName := "database"
	if dbType != "" {
		dbName += "_" + dbType
	}

	fmt.Println("Dumping " + dbName)
	databaseMysqlPass, _ := databaseUrl.User.Password()
	databaseMysqlPort := databaseUrl.Port()
	if len(databaseMysqlPort) < 1 {
		databaseMysqlPort = "3306"
	}
	remoteArgs := []string{
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

	var dumpBuff bytes.Buffer

	dumpCmd.Stdout = writer
	dumpCmd.Stderr = &dumpBuff
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
		fmt.Println("Error output for dump command: ")
		fmt.Println(dumpBuff.String())
	}
	fmt.Println("\tDone writing the " + dbName +" dump file to zip archive")
}