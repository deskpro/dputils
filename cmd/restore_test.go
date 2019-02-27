package cmd

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/deskpro/dputils/util"
	"github.com/spf13/cobra"
	"net/url"
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
	_ = os.Remove(dumpFile)

	dumpFile = getFullBackupDump(filepath.Join("..", "test_mocks"), "database_compressed")

	if dumpFile == "" {
		t.Error("Backup checking failed!")
	}

	_ = os.Remove(dumpFile)
}

func Test_restoreAttachments(t *testing.T) {
	Config.SetDpPath(filepath.Join("..", "test_mocks", "dp_dir"))
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	expectedSql := `SELECT id, save_path, blob_hash FROM blobs WHERE id > \? AND storage_loc = 'fs' ORDER BY id ASC LIMIT 100`
	rows := sqlmock.NewRows([]string{"id", "save_path", "blob_hash"}).AddRow("2", "1/test", "test")
	mock.ExpectQuery(expectedSql).WithArgs(1).WillReturnRows(rows)
	attachUri, _ := filepath.Abs(filepath.Join("..", "test_mocks", "attachments"))
	attachUri = transformAttachUri(attachUri)
	murl := url.URL{
		Scheme:   "mysql",
		User:     url.UserPassword("deskpro", "deskpro"),
		Host:     "localhost",
		Path:     "/deskpro",
		RawPath:  "",
		RawQuery: "",
	}
	mysqlC := util.MysqlConn{murl, db}
	restoreAttachments(mysqlC, attachUri, false, 2)
	attachmentsPath := filepath.Join(Config.DpPath(), "attachments")
	defer os.RemoveAll(attachmentsPath)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func Test_getLstBlobId(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	expectedSqlLastBlobSql := "SELECT id FROM blobs WHERE storage_loc = 'fs' ORDER BY id DESC LIMIT 1 "
	rows := sqlmock.NewRows([]string{"id"}).AddRow("1")
	mock.ExpectQuery(expectedSqlLastBlobSql).WillReturnRows(rows)
	_ = getLastBlobId(db)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}