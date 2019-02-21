package cmd

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"fmt"
	"github.com/deskpro/dputils/util"
	"github.com/hashicorp/go-getter"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

import urlhelper "github.com/hashicorp/go-getter/helper/url"
import _ "github.com/go-sql-driver/mysql"

func init() {
	restoreCmd.Flags().BoolP(
		"interactive",
		"i",
		false,
		`
			Interactive mode. You will be prompted to input values.
		`,
	)

	restoreCmd.Flags().StringP(
		"full-backup",
		"k",
		"",
		`
			 Path to a ZIP containing a database.sql file and an attachments/ folder. 
			 You generate this from any existing Deskpro server by using the 'dputils backup' command.
			 This can be a filesystem path, a HTTP URL, or a S3 URL.
		`,
	)

	restoreCmd.Flags().String(
		"mysql-direct",
		"",
		`
			Connect directly to an existing MySQL server to copy it.
			This is the recommended method because it doesn't require local disk space, the dump and import
			can be streamed over the network.

			If the password is not specified, you will be prompted for it.

			Examples:
				deskpro:mypass@10.1.1.3/deskpro
		`,
	)

	restoreCmd.Flags().String(
		"mysql-direct-audit",
		"",
		`
			An additional information for audit database (it may be stored on a different mysql server).
			If in your new config audit connection config exists we will restore audit db there otherwise we're going 
			to restore audit database into default database.
		`,
	)

	restoreCmd.Flags().String(
		"mysql-direct-system",
		"",
		`
			An additional information for system database (it may be stored on a different mysql server).
			If in your new config system connection config exists we will restore system db there otherwise we're going 
			to restore system database into default database.
		`,
	)

	restoreCmd.Flags().String(
		"mysql-direct-voice",
		"",
		`
			An additional information for voice database (it may be stored on a different mysql server).
			If in your new config voice connection config exists we will restore voice db there otherwise we're going 
			to restore voice database into default database.
		`,
	)

	restoreCmd.Flags().String(
		"attachments",
		"",
		`
			A URL from where attachments can be downloaded. Use this if your attachments are uploaded
			somewhere where they can be downloaded directly. For example, if they are available over HTTP
			or if you have uploaded them to an S3 bucket.

			This can also be a local filesystem path if the files are available locally. Note that files are copied
			(i.e. not moved). If you want to avoid copying, specify the --move-attachments flag.

			If you wish to skip downloading attachments, the use the special string "none". You might do this if attachments
			are not stored in the filesystem (e.g. if they're in the DB or S3).

			Examples:
				https://example.com/deskpro/MRjUXQsZe6h6ESP4hCQReahM56xphf/attachments
				s3::https://s3-eu-west-1.amazonaws.com/bucket/attachments/?aws_access_key_id=xxx&aws_access_key_secret=xxx&aws_access_token=xxx
		`,
	)

	restoreCmd.Flags().Bool(
		"attachments-archive",
		false,
		`
			Set if you want to extract your attachments from an archive
		`,
	)

	restoreCmd.Flags().Bool(
		"move-attachments",
		false,
		`
			If you have specified a local filesystem path with --attachments, this flag can be enabled to make this tool
			MOVE files into place rather than copy them. You might choose to do this if the files are on the local disk already
			and you don't want to consume more disk space.

			If --attachments is not a local filesystem path, then this flag has no effect.
		`,
	)

	restoreCmd.Flags().String(
		"mysql-dump",
		"",
		`
			URI to where a Deskpro database dump exists. If the file is compressed, it will be automatically decompressed.

			Examples:
				https://example.com/deskpro/MRjUXQsZe6h6ESP4hCQReahM56xphf/db.sql.gz
				sftp://user:pass@example.com/tmp/db.sql
				/mnt/db.sql.gz
				D:\db.sql

			If you have a checksum, you may append a query parameter to the URI in the form of type:value to verify
			the download before it's used. This also the a benefit where the download will only happen even upon
			multiple invokations of this command because the file will exist in the tmpdir.

			Examples:
				/mnt/db.sql.gz?checksum=md5:b7d96c89d09d9e204f5fedc4d5d55b21
				/mnt/db.sql.gz?checksum=file:./db.sql.gz.sha256sum

			You can use files from S3 buckets by adding query parameters for credentials:
				s3::https://s3-eu-west-1.amazonaws.com/bucket/foo/db.sql.gz?aws_access_key_id=xxx&aws_access_key_secret=xxx&aws_access_token=xxx
		`,
	)

	restoreCmd.Flags().String(
		"tmpdir",
		"",
		`
			The path on this server to save files to when we need to download them. For example, if using mysql-dump
			then this is where the dump file would be downloaded to.
		`,
	)

	restoreCmd.Flags().Bool(
		"skip-upgrade",
		false,
		`
			Skips the Deskpro upgrade step at the end. You can always run it your later if you want to.
		`,
	)

	restoreCmd.Flags().Bool(
		"reindex-elastic",
		false,
		`
			Schedules Elastic Search indexation.
		`,
	)

	restoreCmd.Flags().Bool(
		"as-test-instance",
		false,
		`
			Disable email processing (make new Deskpro instance to be a test instance)
		`,
	)

	rootCmd.AddCommand(restoreCmd)
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a Deskpro instance to the current server.",
	Long: `
		Provides various options for downloading a database dump and file attachments from an existing
		source, and then imports it into the current server.

		Any option that accepts a remote URI supports the following protocols: http, https, s3.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		tmpdir, _ := cmd.Flags().GetString("tmpdir")
		if len(tmpdir) < 1 {
			tmpdir = os.TempDir()
		}

		log.Info("tmpdir: ", tmpdir)

		isInteractive, _ := cmd.Flags().GetBool("interactive")
		if isInteractive {
			interactiveGatherOptions(cmd)
		}

		dpConfig := Config.ValidateDeskproConfig(cmd)
		destinationMysqlConn := validateDeskpro("database", dpConfig)

		var (
			moveAttachments bool
			attachUri string
			dbDumpLocal string
			sourceMysqlConn util.MysqlConn
		)

		fullBackup, backupDir  := checkFullBackup(cmd, tmpdir)

		if !fullBackup {
			// this one needed to insure we have at least 1 default source connection or dump
			dbDumpLocal, sourceMysqlConn = validateDeskproSource(cmd, tmpdir)
			attachUri, moveAttachments = validateAttachments(cmd, sourceMysqlConn.Conn, tmpdir)
		} else {
			moveAttachments = true
			attachUri = transformAttachUri(filepath.Join(backupDir, "attachments"))
			dbDumpLocal = getFullBackupDump(backupDir, "database")
		}

		restoreDatabase(destinationMysqlConn, sourceMysqlConn, dpConfig, dbDumpLocal, tmpdir)

		if !fullBackup {
			// now let's check we have additional connections like audit, system or voice
			restoreDatabaseAdvanced(cmd, dpConfig, "audit")
			restoreDatabaseAdvanced(cmd, dpConfig, "voice")
			restoreDatabaseAdvanced(cmd, dpConfig, "system")
		} else {
			restoreDatabaseAdvancedDump(backupDir, dpConfig, "audit", tmpdir)
			restoreDatabaseAdvancedDump(backupDir, dpConfig, "voice", tmpdir)
			restoreDatabaseAdvancedDump(backupDir, dpConfig, "system", tmpdir)
		}

		restoreAttachments(destinationMysqlConn, attachUri, moveAttachments)

		doUpgrade(cmd)
		doElasticReset(cmd, destinationMysqlConn)
		markAsTestInstance(cmd, destinationMysqlConn)

		fmt.Println("==========================================================================================")
		fmt.Println("Finished restoring your Deskpro instance. Thank you for using Deskpro.")
		fmt.Println("==========================================================================================")
	},
}

func getFullBackupDump(backupDir string, fileName string) string {
	files, err := ioutil.ReadDir(backupDir)
	if err != nil {
		log.Warning("Failed to get full backup dump file ", err)
		fmt.Println("Failed to get full backup dump file")
		fmt.Println(err)
		os.Exit(1)
	}

	for _, f := range files {
		if strings.HasPrefix(f.Name(), fileName + ".") {
			dumpPath := filepath.Join(backupDir, "dump" + fmt.Sprintf("%d", time.Now().Unix()) + ".sql")
			err := getter.GetFile(dumpPath, filepath.Join(backupDir, f.Name()))
			if err != nil {
				log.Warning("Failed to get full backup dump file ", err)
				fmt.Println("Failed to get full backup dump file")
				fmt.Println(err)
				os.Exit(1)
			}

			return dumpPath
		}
	}

	return ""
}

func checkFullBackup(cmd *cobra.Command, tmpdir string) (bool, string) {

	var (
		backupUri string
		err error
	)
	backupUri, _ = cmd.Flags().GetString("full-backup")
	if backupUri != "" {
		if backupUri, err = filepath.Abs(backupUri); err != nil {
			log.Error("Can't find a full path to dump", backupUri)
			fmt.Println("Backup path is wrong, please check the path for the backup archive carefully")
			fmt.Println(err)
		}
		fmt.Println("==========================================================================================")
		fmt.Println("Detected a full backup flag. Restoring from the full backup archive")
		fmt.Println("==========================================================================================")
		fakename := "backup_archive" + fmt.Sprintf("%d", time.Now().Unix())
		err := getter.GetAny(filepath.Join(tmpdir, fakename), backupUri)
		if err != nil {
			log.Warning("Failed to get full backup archive ", err)
			fmt.Println("Failed to get full backup archive")
			fmt.Println(err)
			os.Exit(1)
		}

		return true, filepath.Join(tmpdir, fakename)
	}

	return false, ""
}

type menuitem struct {
	Id string
	Name string
	Help string
}

func interactiveGatherOptions(cmd *cobra.Command) {

	log.Info("Interactive mode")
	attachUrl := ""
	dumpUrl := ""
	backupUrl := ""

	tpl := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "> [x] {{ .Name | cyan }}",
		Inactive: "  [ ] {{ .Name }}",
		Selected: "{{ .Name | green }}",
		Details: "{{ .Help | yellow }}",
	}

	restoreOptions := []menuitem{
		{"direct", "Direct connection to remote MySQL server", "This establishes a direct MySQL connection over the network. This requires the remote server to be open to network connections, and accessible from this server."},
		{"dump", "Path/URL to a MySQL dump file", "Use this option if you have a MySQL dump (generated from the \"mysqldump\" utility)."},
		{"backup", "Path/URL to a complete backup (ZIP containing both a MySQL dump and attachments)", "Use this option if you have made a complete backup using the \"dputils backup\" utility on your other server."},
	}

	restoreMethodIdx, _, err := (&promptui.Select {
		Label: "Restore Method",
		Items: restoreOptions,
		Templates: tpl,
	}).Run()

	if err != nil {
		log.Error("restore method prompt failed: ", err)
		fmt.Printf("Invalid input: %v\n", err)
		os.Exit(1)
	}

	restoreMethod := restoreOptions[restoreMethodIdx].Id

	log.Info("restoreMethod: ", restoreMethod)

	switch restoreMethod {
	case "direct":
		mysqlUri, err := interactivePromptMysqlUri()
		if err != nil {
			os.Exit(1)
		}
		_ = cmd.Flags().Set("mysql-direct", mysqlUri)

	case "dump":
		dumpUrl, err = (&promptui.Prompt{
			Label: "Path or URL to dump",
		}).Run()

		if err != nil {
			fmt.Printf("Invalid input: %v\n", err)
			os.Exit(1)
		}

		log.Info("dump url: ", dumpUrl)
		_ = cmd.Flags().Set("mysql-dump", dumpUrl)

	case "backup":
		backupUrl, err = (&promptui.Prompt{
			Label: "Path or URL to backup archive",
		}).Run()

		if err != nil {
			fmt.Printf("Invalid input: %v\n", err)
			os.Exit(1)
		}

		log.Info("backup url: ", backupUrl)
		_ = cmd.Flags().Set("full-backup", backupUrl)
	}

	if restoreMethod != "backup" {
		attachOptions := []menuitem{
			{"archive", "Path/URL to an archive containing attachments", "Use this option if you have archived the entire attachments/ directory. E.g. attachments.zip."},
			{"attachments", "Base Path/URL to attachments that can directly accessed", "Use this option if you have uploaded attachments somewhere where they can be downloaded one-by-one. For example, if you copied your attachments/ directory directly to this server, then you could enter the path here."},
			{"skip", "Skip restoring attachments", "This will completely skip the attachments copying step. If there are attachments that exist in the filesystem then they will fail to load on your restored instance."},
		}

		attachMethodIdx, _, err := (&promptui.Select {
			Label: "Restore Method",
			Items: attachOptions,
			Templates: tpl,
		}).Run()

		if err != nil {
			log.Error("attach method prompt failed: ", err)
			fmt.Printf("Invalid input: %v\n", err)
			return
		}

		attachMethod := attachOptions[attachMethodIdx].Id

		log.Info("attachMethod: ", attachMethod)

		switch attachMethod {
		case "archive":
			attachUrl, err = (&promptui.Prompt{
				Label: "Path or URL to archive",
			}).Run()

			if err != nil {
				fmt.Printf("Invalid input: %v\n", err)
				os.Exit(1)
			}

			log.Info("attachments-archive url: ", attachUrl)
			_ = cmd.Flags().Set("attachments-archive", attachUrl)

		case "attachments":
			attachUrl, err = (&promptui.Prompt{
				Label: "Path or URL to attachments",
			}).Run()

			if err != nil {
				fmt.Printf("Invalid input: %v\n", err)
				os.Exit(1)
			}

			log.Info("attachments url: ", attachUrl)
			_ = cmd.Flags().Set("attachments", attachUrl)

		case "skip":
			log.Info("attachments url: none")
			_ = cmd.Flags().Set("attachments", "none")
		}
	}
}

func interactivePromptMysqlUri() (string, error) {
	log.Info("interactivePromptMysqlUri")

	host, err := (&promptui.Prompt{
		Label: "MySQL Host",
	}).Run()

	if err != nil {
		return "", err
	}

	log.Info("interactivePromptMysqlUri Host: ", host)


	portStr, err := (&promptui.Prompt{
		Label: "MySQL Port",
		Default: "3306",
		Validate: func(s string) error {
			_, err = strconv.Atoi(s)
			return err
		},
	}).Run()

	if err != nil {
		return "", err
	}

	port, _ := strconv.Atoi(portStr)

	log.Info("interactivePromptMysqlUri Port: ", port)

	user, err := (&promptui.Prompt{
		Label: "MySQL User",
		Default: "deskpro",
	}).Run()

	if err != nil {
		return "", err
	}

	log.Info("interactivePromptMysqlUri User: ", user)

	password, err := (&promptui.Prompt{
		Label: "MySQL Password",
		Mask: '*',
	}).Run()

	if err != nil {
		return "", err
	}

	log.Info("interactivePromptMysqlUri Password provided")

	dbname, err := (&promptui.Prompt{
		Label: "MySQL DB Name",
	}).Run()

	if err != nil {
		return "", err
	}

	log.Info("interactivePromptMysqlUri DB Name: ", dbname)

	mysqlUriLog := fmt.Sprintf(
		"%s:%s@%s:%d/%s",
		user,
		"***",
		host,
		port,
		dbname,
	)

	mysqlUri := fmt.Sprintf(
		"%s:%s@%s:%d/%s",
		user,
		url.QueryEscape(password),
		host,
		port,
		dbname,
	)

	log.Error("interactivePromptMysqlUri MySQL URI: ", mysqlUriLog)
	fmt.Println("Testing connection...")

	mysqlUrl := util.GetMysqlUrlFromUriString(mysqlUri)
	conn, err := util.GetMysqlConnection(mysqlUrl)

	if err != nil {
		log.Error("interactivePromptMysqlUri DB conn failed: ", err)
		fmt.Println("Failed to connect to remote database: ", err)
		return "", err
	}
	fmt.Println("\tOK")
	_ = conn.Close()

	return mysqlUri, nil
}

func restoreAttachments(destinationMysqlConn util.MysqlConn, attachUri string, moveAttachments bool) {
	realAttachPath := filepath.Join(dpPath, "attachments")

	if attachUri != "none" {
		fmt.Println("==========================================================================================")
		fmt.Println("Restore Attachments")
		fmt.Println("==========================================================================================")

		var (
			err error
			nextStartId int64 = 1
			batch []blobrec
			wg = new(sync.WaitGroup)
		)

		lastId := getLastBlobId(destinationMysqlConn.MysqlUrl)

		for nextStartId < lastId {

			fmt.Println("Batch starting ", nextStartId, "...")
			batch = getNextBlobBatch(destinationMysqlConn.Conn, nextStartId)
			if batch != nil {
				b := batch[len(batch)-1]
				nextStartId = b.id
				wg.Add(1)
				go func(batch []blobrec) {
					defer wg.Done()
					for _, blob := range batch {

						blobPath := strings.Replace(attachUri, "%PATH%", blob.path, 1)
						targetPath := filepath.Join(realAttachPath, filepath.FromSlash(blob.path))
						doSkip := false

						// already exists, check hash
						if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
							doSkip = compareFileHash(targetPath, blob.hash)
						}

						if !doSkip {
							if moveAttachments {
								if _, err := os.Stat(filepath.Dir(targetPath)); os.IsNotExist(err) {
									err := os.MkdirAll(filepath.Dir(targetPath), 0755)
									if err != nil {
										fmt.Println("Failed to create dir for blob: ", blobPath)
										continue
									}
								}
								err = os.Rename(
									blobPath,
									targetPath,
								)
							} else {
								err = getter.GetFile(
									targetPath,
									blobPath,
								)
							}
						}

						if err != nil {
							fmt.Println("Failed to download blob: ", blobPath)
						}
					}
				}(batch)
			}
		}
		wg.Wait()

		fmt.Println("Done all blobs")
	}
}

func restoreDatabaseAdvancedDump(backupDir string, dpConfig map[string]string, dbType string, tmpdir string) {

	var prefix string
	prefix = "database_advanced." + dbType
	dbDumpLocal := getFullBackupDump(backupDir, "database_" + dbType)

	if len(dbDumpLocal) > 1 {
		fmt.Println("Trying to restore database from advanced dump: " + dbType)
		destinationAdvancedMysqlUrl := util.GetMysqlUrlFromConfig(dpConfig, prefix)
		if destinationAdvancedMysqlUrl.User.Username() == "" {
			log.Error("No connection config for database: " + dbType)
			fmt.Println("No connection config for database")
			os.Exit(1)
		}
		destinationAdvancedMysqlConn, err := util.GetMysqlConnectionFromConfig(dpConfig, prefix)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		destinationMysqlConn := util.MysqlConn{destinationAdvancedMysqlUrl, destinationAdvancedMysqlConn}

		restoreDatabase(destinationMysqlConn, util.MysqlConn{}, dpConfig, dbDumpLocal, tmpdir)
	}
}

func restoreDatabaseAdvanced(cmd *cobra.Command, dpConfig map[string]string, dbType string) {

	var (
		flag string
		prefix string
	)
	flag = "mysql-direct-" + dbType

	if cmd.Flags().Changed(flag) {
		prefix = "database_advanced." + dbType

		fmt.Println("Trying to restore database from advanced config: " + dbType)

		advancedSourceConnection := validateDeskproSourceDirect(cmd, flag)
		destinationAdvancedMysqlUrl := util.GetMysqlUrlFromConfig(dpConfig, prefix)
		destinationAdvancedMysqlConn, err := util.GetMysqlConnectionFromConfig(dpConfig, prefix)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		destinationMysqlConn := util.MysqlConn{destinationAdvancedMysqlUrl, destinationAdvancedMysqlConn}
		restoreDatabase(destinationMysqlConn, advancedSourceConnection, dpConfig, "", "")
	}
}

func markAsTestInstance(cmd *cobra.Command, destinationMysqlConn util.MysqlConn) {
	asTestInstance, _ := cmd.Flags().GetBool("as-test-instance")
	if asTestInstance {
		fmt.Println("==========================================================================================")
		fmt.Println("Marking your new Deskpro instance as test instance (email accounts)")
		fmt.Println("==========================================================================================")
		fmt.Println("Disabling email accounts")
		_, err := destinationMysqlConn.Conn.Exec("UPDATE `email_accounts` SET `is_enabled` = 0")
		if err != nil {
			fmt.Println("\tFailed to disable accounts")
		} else {
			fmt.Println("\tOK")
		}
		fmt.Println("Disable url corrections and outgoing emails")

		var re = regexp.MustCompile(`(\$SETTINGS\['disable_url_corrections'\]\s*=)\s*(true|false)(;)`)

		configPath := filepath.Join(Config.DpPath(), "config", "advanced", "config.settings.php")

		bytesRead, err := ioutil.ReadFile(configPath)
		if err != nil {
			fmt.Println("\tCan't read config file.")
			return
		}


		s := string(bytesRead)

		if strings.Contains(s, "disable_url_corrections") && strings.Contains(s, "disable_outgoing_email") {
			s = re.ReplaceAllString(s, "$1 true$3")

			re = regexp.MustCompile(`(\$SETTINGS\['disable_outgoing_email'\]\s*=)\s*(true|false)(;)`)
			s = re.ReplaceAllString(s, "$1 true$3")

			err = ioutil.WriteFile(configPath, []byte(s), 0644)
			if err != nil {
				fmt.Println("\tCan't disable url corrections and outgoing email.")
				return
			}
		} else {
			if file, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0644); err != nil {
				fmt.Println("\tCan't disable url corrections and outgoing email: can not open config file")
			} else {
				if _, err = file.Write([]byte("\r$SETTINGS['disable_url_corrections'] = true;")); err != nil {
					fmt.Println("\tCan't disable url corrections and outgoing email: can't write config file")
					return
				}
				if _, err = file.Write([]byte("\r$SETTINGS['disable_outgoing_email'] = true;")); err != nil {
					fmt.Println("\tCan't disable url corrections and outgoing email: can't write config file")
					return
				}
			}
		}

		fmt.Println("\tOK")
	}
}

func doElasticReset(cmd *cobra.Command, destinationMysqlConn util.MysqlConn) {
	reindexElastic, _ := cmd.Flags().GetBool("reindex-elastic")
	if reindexElastic {
		fmt.Println("==========================================================================================")
		fmt.Println("Scheduling Elasticsearch indexation")
		fmt.Println("==========================================================================================")
		fmt.Println("Setting elastic.requires_reset flag")
		_, err := destinationMysqlConn.Conn.Exec("INSERT INTO `settings` (`name`, `value`) VALUES ('elastica.requires_reset', 1) ON DUPLICATE KEY UPDATE `value` = 1")
		if err != nil {
			fmt.Println("\tFailed to set flag")
		} else {
			fmt.Println("\tOK")
		}
		fmt.Println("Updating Elastic indexer status")
		_, err2 := destinationMysqlConn.Conn.Exec("DELETE FROM `datastore` WHERE `name` = 'sys.es_indexer'")
		if err2 != nil {
			fmt.Println("\tFailed to reset indexer status")
		} else {
			fmt.Println("\tOK")
		}

		if err != nil || err2 != nil {
			fmt.Println("Failed to schedule Elastic reindex")
		} else {
			fmt.Println("Scheduled Elasticsearch reindexation for next cron start")
		}
	}
}

func doUpgrade(cmd *cobra.Command) {
	skipUpgrade, _ := cmd.Flags().GetBool("skip-upgrade")

	fmt.Println("==========================================================================================")
	fmt.Println("Running Deskpro upgrade")
	fmt.Println("==========================================================================================")

	if skipUpgrade {
		fmt.Println("Skipping upgrade, --skip-upgrade flag specified")
		return
	}

	phpPath := Config.PhpPath()
	upgradeCmd := exec.Command(
		phpPath,
		filepath.Join(Config.DpPath(), "bin", "console"),
		"dp:upgrade",
	)

	var buff bytes.Buffer
	upgradeCmd.Stdout = &buff
	upgradeCmd.Stderr = &buff

	_ = upgradeCmd.Start()

	err := upgradeCmd.Wait()

	if err != nil {
		fmt.Println("Deskpro upgrade failed!")
		fmt.Println(buff.String())
		fmt.Println(err)
	} else {
		fmt.Println("Deskpro upgrade success")
		fmt.Println(buff.String())
	}
}



func validateDeskpro(prefix string, dpConfig map[string]string) util.MysqlConn {
	var (
		localDbConn          *sql.DB
		localDbUrl           url.URL
		destinationMysqlConn util.MysqlConn
	)

	localDbUrl = util.GetMysqlUrlFromConfig(dpConfig, prefix)
	localDbConn, err := util.GetMysqlConnectionFromConfig(dpConfig, prefix)
	if err != nil {
		log.Error("Failed to connect to db ", err)
		fmt.Println("The database details contained in config.database.php do not work. This is the error:")
		fmt.Println(err)
		fmt.Println("Please correct the database configuration and then try again.")
		os.Exit(1)
	}

	destinationMysqlConn = util.MysqlConn{localDbUrl, localDbConn}

	res, err := localDbConn.Query("SHOW TABLES")
	if err != nil {
		log.Warning("Failed to SHOW TABLES on local db ", err)
		fmt.Println("The database details contained in config.database.php do not work. This is the error:")
		fmt.Println(err)
		fmt.Println("Please correct the database configuration and then try again.")
		os.Exit(1)
	}

	if res.Next() {
		log.Info("local db has tables")

		// this checks for a count of settings matches the settings that get set upon install
		// if these arent there, then we can assume its a new instance that hasnt even been configured yet
		res, err = localDbConn.Query("SELECT COUNT(*) FROM settings WHERE name IN ('admin_has_loaded', 'core.license', 'core.setup_initial')")
		settingCount := 0

		if err != nil {
			log.Warning("scanning for settings failed ", err)
			// an error (i.e. maybe tbale didnt exist) lets just use same handling as if its an install
			settingCount = 3
		} else {
			if res.Next() {
				_ = res.Scan(&settingCount)
			}
		}

		if settingCount == 3 {
			log.Info("found some records that indicate real install")
			fmt.Println("The local db already contains tables. If this is a new server, then it might simply be the default demo installation.")
			fmt.Println("You can wipe the installation with the following command: ")
			fmt.Println("")
			fmt.Println(Config.PhpPath(), " ", filepath.Join(Config.DpPath(), "bin", "console"), " install:clean --keep-config")
			fmt.Println("")
			os.Exit(1)
		}
	}


	return destinationMysqlConn
}

func validateDeskproSource(cmd *cobra.Command, tmpdir string) (string, util.MysqlConn) {
	//------------------------------
	// Database conn or dump
	//------------------------------
	fmt.Println("After you have wiped the existing installation, try the restore again.")

	fmt.Println("")
	fmt.Println("==========================================================================================")
	fmt.Println("Your existing database to restore")
	fmt.Println("==========================================================================================")

	var (
		dbDumpLocal string
		sourceConn util.MysqlConn
	)
	if cmd.Flags().Changed("mysql-direct") {
		sourceConn = validateDeskproSourceDirect(cmd, "mysql-direct")
	} else if cmd.Flags().Changed("mysql-dump") {
		dbDumpLocal = validateDeskproSourceDump(cmd, tmpdir)
	} else {
		log.Info("no --mysql-direct or --mysql-dump specified")
		fmt.Println("We need a way to get the database. You can use either --mysql-direct or --mysql-dump. Check --help for more information.")
		os.Exit(1)
	}

	return dbDumpLocal, sourceConn
}

// validateDeskproDestination checks if destination database is ready to accept database dump which may be either
// a file or a direct mysql connection to dump all tables with mysqldump
func validateDeskproSourceDirect(cmd *cobra.Command, flag string) util.MysqlConn {

	//var attachLocal string
	var sourceConn util.MysqlConn

	mysqlUrl, conn := doValidateDeskproSource(cmd, flag)
	sourceConn = util.MysqlConn{mysqlUrl, conn}

	return sourceConn
}

func validateDeskproSourceDump(cmd *cobra.Command, tmpdir string) (string) {
	var (
		dbDumpLocal string
		err error
	)

	dumpUri, _ := cmd.Flags().GetString("mysql-dump")

	if dumpUri, err = filepath.Abs(dumpUri); err != nil {
		log.Error("Can't find a full path to dump", dumpUri)
		fmt.Println("Can't find a full path to dump, please check your --mysql-dump option carefully")
		fmt.Println(err)
		os.Exit(1)
	}

	log.Info("--mysql-dump = ", dumpUri)

	fmt.Println("Using database dump from: ", dumpUri)

	dbDumpLocal = filepath.Join(tmpdir, "db.sql")
	log.Info("save to", dbDumpLocal)

	fmt.Println("Downloading to temp file: ", dbDumpLocal)

	err = getter.GetFile(dbDumpLocal, dumpUri)
	if err != nil {
		log.Warning("download dump failed: ", err)
		fmt.Println("Failed to download database dump: ", err)
		os.Exit(1)
	}

	fmt.Println("\tOK")

	return dbDumpLocal
}

func doValidateDeskproSource(cmd *cobra.Command, flag string) (url.URL, *sql.DB) {
	var mysqlUrl url.URL
	var conn *sql.DB
	var err error

	mysqlUri, _ := cmd.Flags().GetString(flag)

	log.Info("--mysq-client = ", mysqlUri)

	fmt.Println("Using direct MySQL connection to: ", mysqlUri)
	fmt.Println("Testing connection...")

	mysqlUrl = util.GetMysqlUrlFromUriString(mysqlUri)
	conn, err = util.GetMysqlConnection(mysqlUrl)

	if err != nil {
		fmt.Println("Failed to connect to remote database")
		os.Exit(1)
	}
	fmt.Println("\tOK")

	return mysqlUrl, conn
}

// validateAttachments will perform general attachments validation and will return attachUri string which indicates
// where attachments are stored
func validateAttachments(cmd *cobra.Command, conn *sql.DB, tmpdir string) (string, bool) {
	//------------------------------
	// Attachments
	//------------------------------

	fmt.Println("==========================================================================================")
	fmt.Println("Attachments")
	fmt.Println("==========================================================================================")

	attachUri, _ := cmd.Flags().GetString("attachments")
	if len(attachUri) < 1 {
		log.Info("no --attachments specified")
		fmt.Println("You must specify a path for attachments with --attachments. See --help for more information.")
		os.Exit(1)
	}

	var err error

	if attachUri, err = filepath.Abs(attachUri); err != nil {
		log.Error("Can't find a full path to dump", attachUri)
		fmt.Println("Can't find a full path to attachments, please check your --attachments option carefully")
		fmt.Println(err)
		os.Exit(1)
	}

	aUrl, err := url.Parse(attachUri)
	if err != nil {
		log.Info("--attachments contains wrong URI")
		fmt.Println("You must specify a correct path for attachments with --attachments. See --help for more information.")
		os.Exit(1)
	}
	moveAttachments := false
	if aUrl.Scheme == "" || aUrl.Scheme == "file" {
		attachUri = aUrl.Path
		moveAttachments, _ = cmd.Flags().GetBool("move-attachments")
	}

	archive, _ := cmd.Flags().GetBool("attachments-archive")

	if !archive {
		// try to detect archive from attachUri
		// that was copy-pasted directly from go-getter
		archive = detectArchive(attachUri, tmpdir)
	}

	if archive {
		fakename := "attachments" + fmt.Sprintf("%d", time.Now().Unix())
		err := getter.GetAny(filepath.Join(tmpdir, fakename), attachUri)
		if err != nil {
			log.Info("failed to load attachments archive: ", err)
			fmt.Println("Trying to download attachments archive failed: ", err)
			os.Exit(1)
		}
		attachUri = filepath.Join(tmpdir, fakename)
		// just to save space
		// anyway we're going to download/copy archive in temp, so why keep files there?
		moveAttachments = true
	}

	if attachUri != "none" {
		attachUri = transformAttachUri(attachUri)
		log.Info("--attachments is ", attachUri)
	} else {
		fmt.Println("none -- skipping attachments")
	}

	// Verify a file just to validate
	if conn != nil && attachUri != "none" {
		res, err := conn.Query("SELECT save_path FROM blobs WHERE storage_loc = 'fs' ORDER BY id DESC LIMIT 1")
		if err != nil {
			log.Info("failed blob select: ", err)
			fmt.Println("Trying to select an attachment record from the database failed: ", err)
			os.Exit(1)
		}

		var savePath string

		if res.Next() {
			if err := res.Scan(&savePath); err != nil {
				log.Info("failed blob scan: ", err)
				fmt.Println("Trying to select an attachment record from the database failed: ", err)
				os.Exit(1)
			}
		}

		_ = res.Close()

		// if there are no fs blobs, then there are no attachments
		if len(savePath) < 1 {
			log.Info("no fs blobs, attachUri = none")
			fmt.Println("We detected no filesystem attachments in the database, so there are no attachments to copy over.")
			fmt.Println("You can use --attachments=none to skip this step in future.")
			attachUri = "none"

			// try to download it
		} else {
			fmt.Println("Testing attachments option...")

			expectFile := strings.Replace(attachUri, "%PATH%", savePath, 1)
			tmpFile := filepath.Join(tmpdir, "test_file_dl")

			err := getter.GetFile(tmpFile, expectFile)
			if err != nil {
				log.Info("Failed to download test file: ", err, ". Expected: ", expectFile)
				fmt.Println("Failed to download test file: ", err, ". Expected: ", expectFile)
				os.Exit(1)
			}

			fmt.Println("\tOK")
		}
	}

	return attachUri, moveAttachments
}

func transformAttachUri(attachUri string) string {
	// turns a path into a suitable uri with placeholder string
	// e.g. C:\foo\bar?some_option=value -> C:/foo/bar/%PATH%?some_option
	// so we can have a single string and get the path easily with a string replace

	fmt.Println("Attachments will be loaded from: ", attachUri)

	attachUri = strings.Replace(attachUri, "\\", "/", -1)

	if !strings.Contains(attachUri, "%PATH%") {
		re := regexp.MustCompile("/?(\\?.*)?$")
		attachUri = re.ReplaceAllString(attachUri, "/%PATH%$1")
	}

	return attachUri
}

func detectArchive(uri string, tmpdir string) bool{
	c := getter.Client{
		Src:     uri,
		Dst:     filepath.Join(tmpdir, "fake"),
		Mode:    getter.ClientModeAny,
	}
	c.Decompressors = getter.Decompressors

	u, err := urlhelper.Parse(uri)
	if err != nil {
		return false
	}
	archive := ""
	matchingLen := 0
	for k := range c.Decompressors {
		if strings.HasSuffix(u.Path, "."+k) && len(k) > matchingLen {
			archive = k
			matchingLen = len(k)
		}
	}

	return archive != ""
}

// restoreDatabse performs actual database restore from remote db to local db
// returns nothing
func restoreDatabase(destinationMysqlConn util.MysqlConn, sourceMysqlConn util.MysqlConn, dpConfig map[string]string, dbDumpLocal string, tmpdir string) {
	fmt.Println("==========================================================================================")
	fmt.Println("Restore Database")
	fmt.Println("==========================================================================================")

	fmt.Println("Clearing existing database...")

	tableList, _ := destinationMysqlConn.Conn.Query("SHOW TABLES")

	_, _ = destinationMysqlConn.Conn.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for tableList.Next() {
		var tableName string
		_ = tableList.Scan(tableName)

		if len(tableName) > 0 {
			_, _ = destinationMysqlConn.Conn.Exec("DROP TABLE `" + tableName + "`")
		}
	}
	_, _ = destinationMysqlConn.Conn.Exec("SET FOREIGN_KEY_CHECKS = 1")

	fmt.Println("\tOK")

	localMysqlPass, _ := destinationMysqlConn.MysqlUrl.User.Password()
	localMysqlPort := destinationMysqlConn.MysqlUrl.Port()
	if len(localMysqlPort) < 1 {
		localMysqlPort = "3306"
	}

	mysqlBin := dpConfig["paths.mysql_path"]
	mysqlDumpBin := dpConfig["paths.mysqldump_path"]

	localArgs := []string{
		"-h", destinationMysqlConn.MysqlUrl.Host,
		"--port", localMysqlPort,
		"-u", destinationMysqlConn.MysqlUrl.User.Username(),
	}
	if localMysqlPass != "" {
		localArgs = append(localArgs, "--password="+localMysqlPass)
	}
	archive := detectArchive(dbDumpLocal, tmpdir)
	if archive {
		newPath := filepath.Join(tmpdir, "deskpro_database.sql" + fmt.Sprintf("%d", time.Now().Unix()))
		err := getter.GetFile(newPath, dbDumpLocal)
		if err != nil {
			log.Warning("Failed to unarchive backup file", err)
			fmt.Println("Failed to unarchive backup file")
			fmt.Println(err)
			os.Exit(1)
		}
		dbDumpLocal = newPath
	}
	localArgs = append(localArgs, strings.TrimLeft(destinationMysqlConn.MysqlUrl.Path, "/"))

	if len(dbDumpLocal) > 1 {

		dumpFile, err := os.Open(dbDumpLocal)
		defer dumpFile.Close()
		if err != nil {
			log.Error("Couldn't open dump file: ", err)
			fmt.Println("Couldn't open dump file")
			fmt.Println(err)
			os.Exit(1)
		}
		b := make([]byte, 1024*100)
		_, err = dumpFile.Read(b)
		if err != nil {
			log.Error("Couldn't read dump file: ", err)
			fmt.Println("Couldn't read dump file")
			fmt.Println(err)
			os.Exit(1)
		}
		if !strings.Contains(string(b), "agent_activity") {
			log.Error("The dump file seems to be broken")
			fmt.Println("The dump file seems to be broken, we can't find correct SQL dump for Deskpro tables")
			fmt.Println(err)
			os.Exit(1)
		}

		fmt.Println("Restoring from database dump (this may take a while)...")

		localArgs = append(localArgs, "-e", "source " + dbDumpLocal)

		out, err := exec.Command(
			mysqlBin,
			localArgs...,
		).CombinedOutput()
		if err != nil {
			fmt.Println(string(out))
			fmt.Println("Failed to restore mysql dump: ", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Restoring from mysqldump (this may take a while)...")

		remoteMysqlPass, _ := sourceMysqlConn.MysqlUrl.User.Password()
		remoteMysqlPort := sourceMysqlConn.MysqlUrl.Port()
		if len(remoteMysqlPort) < 1 {
			remoteMysqlPort = "3306"
		}

		remoteArgs := []string{
			"-h", sourceMysqlConn.MysqlUrl.Host,
			"--port", remoteMysqlPort,
			"-u", sourceMysqlConn.MysqlUrl.User.Username(),
			"-C",
		}
		if remoteMysqlPass != "" {
			remoteArgs = append(remoteArgs, "--password=remoteMysqlPass")
		}
		remoteArgs = append(remoteArgs, strings.TrimLeft(sourceMysqlConn.MysqlUrl.Path, "/"))

		reader, writer, err := os.Pipe()
		if err != nil {
			fmt.Println(err)
			fmt.Println("IO error -- failed to get pipes: ", err)
			os.Exit(1)
		}

		dumpCmd := exec.Command(
			mysqlDumpBin,
			remoteArgs...
		)

		importCmd := exec.Command(
			mysqlBin,
			localArgs...
		)

		dumpCmd.Stdout = writer
		importCmd.Stdin = reader

		var buff bytes.Buffer
		importCmd.Stdout = &buff
		importCmd.Stderr = &buff

		_ = dumpCmd.Start()
		_ = importCmd.Start()
		_ = dumpCmd.Wait()
		_ = writer.Close()
		_ = reader.Close()
		err = importCmd.Wait()

		if err != nil {
			fmt.Println(buff.String())
			fmt.Println("Failed to restore mysql dump: ", err)
			os.Exit(1)
		}
	}

	fmt.Println("\tOK")
}

type blobrec struct {
	id   int64
	path string
	hash string
}



func compareFileHash(filePath string, expectHash string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}

	return fmt.Sprintf("%x", md5.Sum(nil)) == expectHash
}

func getLastBlobId(murl url.URL) int64 {
	db, err := util.GetMysqlConnection(murl)
	if err != nil {
		panic(err)
	}

	res, err := db.Query("SELECT id FROM blobs WHERE storage_loc = 'fs' ORDER BY id DESC LIMIT 1 ")

	if err != nil {
		panic(err)
	}

	defer res.Close()

	if res.Next() {
		var id int64
		err = res.Scan(&id)
		if err != nil {
			panic(err)
		}

		return id
	} else {
		return 0
	}
}

func getNextBlobBatch(db *sql.DB, startId int64) []blobrec {

	res, err := db.Query(`
		SELECT id, save_path, blob_hash
		FROM blobs
		WHERE
			id > ?
			AND storage_loc = 'fs'
		ORDER BY id ASC
		LIMIT 100
	`, startId)

	defer res.Close()

	if err != nil {
		panic(err)
	}

	var recs []blobrec

	for res.Next() {
		var r blobrec
		err = res.Scan(&r.id, &r.path, &r.hash)
		if err != nil {
			panic(err)
		}

		recs = append(recs, r)
	}

	return recs
}


