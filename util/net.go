package util

import (
	"database/sql"
	"fmt"
	"github.com/manifoldco/promptui"
	"net/url"
	"os"
)

type MysqlConn struct {
	MysqlUrl url.URL
	Conn     *sql.DB
}

func GetMysqlUrlFromUriString(uri string) url.URL {
	murl, err := url.Parse("mysql://" + uri)

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
			Label: "MySQL Password (just hit enter if empty)",
			Mask:  '*',
		}

		pass, err = prompt.Run()
	}

	return *(&url.URL{
		Scheme:   "mysql",
		User:     url.UserPassword(murl.User.Username(), pass),
		Host:     murl.Host,
		Path:     murl.Path,
		RawPath:  murl.RawPath,
		RawQuery: murl.RawQuery,
	})
}

func GetMysqlConnection(murl url.URL) (*sql.DB, error) {
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

func GetMysqlConnectionFromConfig(dpConfig map[string]string, prefix string) (*sql.DB, error) {
	return GetMysqlConnection(GetMysqlUrlFromConfig(dpConfig, prefix))
}

func GetMysqlUrlFromConfig(dpConfig map[string]string, prefix string) url.URL {
	murl, err := url.Parse(
		"mysql://" + dpConfig[prefix + ".user"] +
			":" + dpConfig[prefix + ".password"] +
			"@" + dpConfig[prefix + ".host"] +
			"/" + dpConfig[prefix + ".dbname"])

	if err != nil {
		fmt.Println("Database connection in config.database.php is invalid or corrupt")
		fmt.Println(err)
		os.Exit(1)
	}

	return *murl
}
