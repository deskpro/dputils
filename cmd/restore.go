package cmd

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"fmt"
	"github.com/golang/glog"
	"github.com/hashicorp/go-getter"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

import _ "github.com/go-sql-driver/mysql"

func init() {
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

			You can use files from S3 buckets by adding query paramters for credentials:
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

	rootCmd.AddCommand(restoreCmd)
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a Deskpro instance to the current server.",
	Long: `
		Provides various options for downloading a database dump and file attachments from an existing
		source, and then imports it into the current server.

		Any option that accepts a remote URI supports the following protocols: http, https, ftp, sftp.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		tmpdir, _ := cmd.Flags().GetString("tmpdir")
		if len(tmpdir) < 1 {
			tmpdir = os.TempDir()
		}

		glog.V(1).Info("tmpdir: ", tmpdir)

		//------------------------------
		// Validate current deskpro
		//------------------------------

		var localDbConn *sql.DB
		var localDbUrl url.URL

		fmt.Println("=================================================")
		fmt.Println("This Deskpro server")
		fmt.Println("=================================================")

		fmt.Println("We will restore data onto this current server. Deskpro is installed here: ")
		fmt.Println("\tDeskpro Path: ", GetDeskproPath())
		fmt.Println("\tConfig Path: ", filepath.Join(GetDeskproPath(), "config"))

		dpConfig, err := GetDeskproConfig()

		if err != nil {
			glog.Error("Failed to read config ", err)
			fmt.Println("We failed to read the Deskpro config files. Are they there?")
			fmt.Println("To start fresh, you can install clean config files with this command:")
			fmt.Println("")
			fmt.Println(GetPhpPath(), " ", filepath.Join(GetDeskproPath(), "bin", "console"), " install:fresh-config")
			fmt.Println("")
			fmt.Println("After config files are inserted, you will need to modify the config.database.php file with your database details.")
			os.Exit(1)
		} else {
			localDbUrl = getMysqlUrlFromConfig(dpConfig)
			localDbConn, err = getMysqlConnectionFromConfig(dpConfig)
			if err != nil {
				glog.Error("Failed to connect to db ", err)
				fmt.Println("The database details contained in config.database.php do not work. This is the error:")
				fmt.Println(err)
				fmt.Println("Please correct the database configuration and then try again.")
				os.Exit(1)
			}

			res, err := localDbConn.Query("SHOW TABLES")
			if err != nil {
				glog.Warning("Failed to SHOW TABLES on local db ", err)
				fmt.Println("The database details contained in config.database.php do not work. This is the error:")
				fmt.Println(err)
				fmt.Println("Please correct the database configuration and then try again.")
				os.Exit(1)
			}

			if res.Next() {
				glog.Info("local db has tables")

				// this checks for a count of settings matches the settings that get set upon install
				// if these arent there, then we can assume its a new instance that hasnt even been configured yet
				res, err = localDbConn.Query("SELECT COUNT(*) FROM settings WHERE name IN ('admin_has_loaded', 'core.license', 'core.setup_initial')")
				settingCount := 0

				if err != nil {
					glog.Warning("scanning for settings failed ", err)
					// an error (i.e. maybe tbale didnt exist) lets just use same handling as if its an install
					settingCount = 3
				} else {
					if res.Next() {
						_ = res.Scan(&settingCount)
					}
				}

				if settingCount == 3 {
					glog.Info("found some records that indicate real install")
					fmt.Println("The local db already contains tables. If this is a new server, then it might simply be the default demo installation.")
					fmt.Println("You can wipe the installation with the following command: ")
					fmt.Println("")
					fmt.Println(GetPhpPath(), " ", filepath.Join(GetDeskproPath(), "bin", "console"), " install:clean --keep-config")
					fmt.Println("")
					fmt.Println("After you have wiped the existing installation, try the restore again.")
					os.Exit(1)
				}
			}
		}

		//------------------------------
		// Database conn or dump
		//------------------------------

		fmt.Println("")
		fmt.Println("=================================================")
		fmt.Println("Your existing database to restore")
		fmt.Println("=================================================")

		//var attachLocal string
		var dbDumpLocal string
		var mysqlUrl url.URL

		mysqlUri, _  := cmd.Flags().GetString("mysql-direct")

		var conn *sql.DB

		if cmd.Flags().Changed("mysql-direct") {
			glog.Info("--mysq-client = ", mysqlUri)

			fmt.Println("Using direct MySQL connection to: ", mysqlUri)
			fmt.Println("Testing connection...")

			mysqlUrl = getMysqlUrlFromUriString(mysqlUri)
			conn, err = getMysqlConnection(mysqlUrl)

			if err != nil {
				fmt.Println("Failed to connect to remote database")
				os.Exit(1)
			}
			fmt.Println("\tOK")

		} else if cmd.Flags().Changed("mysql-dump") {
			dumpUri, _ := cmd.Flags().GetString("mysql-dump")
			glog.Info("--mysql-dump = ", dumpUri)

			fmt.Println("Using database dump from: ", dumpUri)

			dbDumpLocal = filepath.Join(tmpdir, "db.sql")
			glog.Info("save to", dbDumpLocal)

			fmt.Println("Downloading to temp file: ", dbDumpLocal)

			err := getter.GetFile(dbDumpLocal, dumpUri)
			if err != nil {
				glog.Warning("download dump failed: ", err)
				fmt.Println("Failed to download database dump: ", err)
				os.Exit(1)
			}

			fmt.Println("\tOK")
		} else {
			glog.Info("no --mysql-direct or --mysql-dump specified")
			fmt.Println("We need a way to get the database. You can use either --mysql-direct or --mysql-dump. Check --help for more information.")
			os.Exit(1)
		}

		//------------------------------
		// Attachments
		//------------------------------

		fmt.Println("=================================================")
		fmt.Println("Attachments")
		fmt.Println("=================================================")

		//doMoveAttach, _ := cmd.Flags().GetBool("move-attachments")

		attachUri, _ := cmd.Flags().GetString("attachments")
		if len(attachUri) < 1 {
			glog.Info("no --attachments specified")
			fmt.Println("You must specify a path for attachments with --attachments. See --help for more information.")
			os.Exit(1)
		}

		if attachUri != "none" {
			// turns a path into a suitable uri with placeholder string
			// e.g. C:\foo\bar?some_option=value -> C:/foo/bar/%PATH%?some_option
			// so we can have a single string and get the path easily with a string replace

			fmt.Println("Attachments will be loaded from: ", attachUri)

			attachUri = strings.Replace(attachUri, "\\", "/", -1)

			if !strings.Contains(attachUri, "%PATH%") {
				re := regexp.MustCompile("/?(\\?.*)?$")
				attachUri = re.ReplaceAllString(attachUri, "/%PATH%$1")
			}

			glog.Info("--attachments is ", attachUri)
		} else {
			fmt.Println("none -- skipping attachments")
		}

		// Verify a file just to validate
		if conn != nil && attachUri != "none" {
			res, err := conn.Query("SELECT save_path FROM blobs WHERE storage_loc = 'fs' ORDER BY id DESC LIMIT 1")
			if err != nil {
				glog.Info("failed blob select: ", err)
				fmt.Println("Trying to select an attachment record from the database failed: ", err)
				os.Exit(1)
			}

			var savePath string

			if res.Next() {
				if err := res.Scan(&savePath); err != nil {
					glog.Info("failed blob scan: ", err)
					fmt.Println("Trying to select an attachment record from the database failed: ", err)
					os.Exit(1)
				}
			}

			_ = res.Close()

			// if there are no fs blobs, then there are no attachments
			if len(savePath) < 1 {
				glog.Info("no fs blobs, attachUri = none")
				fmt.Println("We detected no filesystem attachments in the database, so there are no attachments to copy over.")
				fmt.Println("You can use --attachments=none to skip this step in future.")
				attachUri = "none"

			// try to download it
			} else {
				fmt.Println("Testing attachments option...")

				expectFile := strings.Replace(attachUri, "%PATH%", savePath, 1)
				tmpFile := filepath.Join(tmpdir, "test_file_dl")

				err := getter.GetFile(expectFile, tmpFile)
				if err != nil {
					glog.Info("Failed to download test file: ", err, ". Expected: ", expectFile)
					fmt.Println("Failed to download test file: ", err, ". Expected: ", expectFile)
					os.Exit(1)
				}

				fmt.Println("\tOK")
			}
		}

		//------------------------------
		// Restore database
		//------------------------------

		fmt.Println("=================================================")
		fmt.Println("Restore Database")
		fmt.Println("=================================================")

		fmt.Println("Clearing existing database...")

		tableList, _ := localDbConn.Query("SHOW TABLES")

		_, _ = localDbConn.Exec("SET FOREIGN_KEY_CHECKS = 0")
		for tableList.Next() {
			var tableName string
			_ = tableList.Scan(tableName)

			if len(tableName) > 0 {
				_, _ = localDbConn.Exec("DROP TABLE `" + tableName + "`")
			}
		}
		_, _ = localDbConn.Exec("SET FOREIGN_KEY_CHECKS = 1")

		fmt.Println("\tOK")

		localMysqlPass, _ := localDbUrl.User.Password()
		localMysqlPort := mysqlUrl.Port()
		if len(localMysqlPort) < 1 {
			localMysqlPort = "3306"
		}

		mysqlBin := dpConfig["paths.mysql_path"]
		mysqlDumpBin := dpConfig["paths.mysqldump_path"]

		localArgs := []string{
			"-h", localDbUrl.Host,
			"--port", localMysqlPort,
			"-u", localDbUrl.User.Username(),
		}
		if localMysqlPass != "" {
			localArgs = append(localArgs, "-p", localMysqlPass)
		}
		localArgs = append(localArgs, strings.TrimLeft(localDbUrl.Path, "/"))

		if len(dbDumpLocal) > 1 {
			fmt.Println("Restoring from database dump (this may take a while)...")

			out, err := exec.Command(
				mysqlBin,
				localArgs...,
			).CombinedOutput()
			if err != nil {
				fmt.Println(out)
				fmt.Println("Failed to restore mysql dump: ", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Restoring from mysqldump (this may take a while)...")

			remoteMysqlPass, _ := localDbUrl.User.Password()
			remoteMysqlPort := mysqlUrl.Port()
			if len(remoteMysqlPort) < 1 {
				remoteMysqlPort = "3306"
			}

			remoteArgs := []string{
				"-h", mysqlUrl.Host,
				"--port", remoteMysqlPort,
				"-u", mysqlUrl.User.Username(),
				"-C",
			}
			if remoteMysqlPass != "" {
				remoteArgs = append(remoteArgs, "-p", remoteMysqlPass)
			}
			remoteArgs = append(remoteArgs, strings.TrimLeft(mysqlUrl.Path, "/"))

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


		//------------------------------
		// Restore files
		//------------------------------

		realAttachPath := filepath.Join(dpPath, "attachments")

		if attachUri != "none" {
			fmt.Println("=================================================")
			fmt.Println("Restore Attachments")
			fmt.Println("=================================================")

			lastId := getLastBlobId(localDbUrl)

			var nextStartId int64
			nextStartId = 1

			var batch []blobrec

			for nextStartId < lastId {

				fmt.Println("Batch starting ", nextStartId, "...")

				batch = getNextBlobBatch(localDbUrl, nextStartId)

				for _, blob := range batch {
					blobPath := strings.Replace(attachUri, "%PATH%", blob.path, 1)
					targetPath := filepath.Join(realAttachPath, filepath.FromSlash(blob.path))
					doSkip := false

					// already exists, check hash
					if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
						doSkip = compareFileHash(targetPath, blob.hash)
					}

					if !doSkip {
						err = getter.GetFile(
							targetPath,
							blobPath,
						)
					}

					if err != nil {
						fmt.Println("Failed to download blob: ", blobPath)
					}
				}
			}

			fmt.Println("Done all blobs")
		}

		// TODO handle move-attachments option

		//------------------------------
		// Run upgrade
		//------------------------------

		doSkipUpgrade, _ := cmd.Flags().GetBool("skip-upgrade")

		if !doSkipUpgrade {
			//TODO perform upgrade
		}

		//TODO set flag that makes ES re-index
		//TODO handle setting flags to disable email (for use with test instances)
	},
}

type blobrec struct {
	id int64
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
	db, err := getMysqlConnection(murl)
	if err != nil {
		panic(err)
	}

	res, err := db.Query("SELECT id FROM blobs ORDER BY id DESC LIMIT 1")

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

func getNextBlobBatch(murl url.URL, startId int64) []blobrec {
	db, err := getMysqlConnection(murl)
	if err != nil {
		panic(err)
	}

	endId := (startId+1000)-1

	res, err := db.Query(`
		SELECT id, save_path, blob_hash
		FROM blobs
		WHERE
			id BETWEEN ? AND ?
			AND storage_loc = 'fs'
		ORDER BY id ASC
		LIMIT 1000
	`, startId, endId)

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

func getMysqlUrlFromUriString(uri string) url.URL {
	murl, err := url.Parse("mysql://"+uri)

	if err != nil {
		fmt.Println("--mysql-direct: Invalid MySQL URI string")
		fmt.Println(err)
		os.Exit(1)
	}

	if len(murl.User.Username()) < 1 {
		fmt.Println("--mysql-direct: Username is missing")
		os.Exit(1)
	}

	//var pass string
	pass, _ := murl.User.Password()

	if len(pass) < 1 {
		prompt := promptui.Prompt{
			Label:    "MySQL Password (just hit enter if empty)",
			Mask:     '*',
		}

		pass, err = prompt.Run()
	}

	return *(&url.URL{
		Scheme: "mysql",
		User: url.UserPassword(murl.User.Username(), pass),
		Host: murl.Host,
		Path: murl.Path,
		RawPath: murl.RawPath,
		RawQuery: murl.RawQuery,
	})
}

func getMysqlConnection(murl url.URL) (*sql.DB, error) {
	pass, _ := murl.User.Password()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)%s", murl.User.Username(), pass, murl.Host, murl.Path)

	db, err := sql.Open(
		"mysql",
		dsn,
	)

	if err != nil {
		return nil, err
	}

	_, err = db.Query("SELECT 1")

	if err != nil {
		return nil, err
	}

	return db, nil
}

func getMysqlConnectionFromConfig(dpConfig map[string]string) (*sql.DB, error) {
	return getMysqlConnection(getMysqlUrlFromConfig(dpConfig))
}

func getMysqlUrlFromConfig(dpConfig map[string]string) url.URL {
	murl, err := url.Parse("mysql://" + dpConfig["database.user"] + ":" + dpConfig["database.password"] + "@" + dpConfig["database.host"] + "/" + dpConfig["database.dbname"])

	if err != nil {
		fmt.Println("Database connection in config.database.php is invalid or corrupt")
		fmt.Println(err)
		os.Exit(1)
	}

	return *murl
}