package util

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestGetMysqlUrlFromUriString(t *testing.T) {
	expectedUrl := *(&url.URL{
		Scheme:   "mysql",
		User:     url.UserPassword("deskpro", "deskpro"),
		Host:     "localhost",
		Path:     "/deskpro",
		RawPath:  "",
		RawQuery: "",
	})


	mysqlUri := fmt.Sprintf(
		"%s:%s@%s:%d/%s",
		"deskpro",
		url.QueryEscape("deskpro"),
		"localhost",
		3306,
		"deskpro",
	)

	actualUrl := GetMysqlUrlFromUriString(mysqlUri)

	fmt.Println(actualUrl.RawQuery, actualUrl.RawPath)

	if !reflect.DeepEqual(actualUrl, expectedUrl) {
		t.Error("Expected and actual configs are not equal")
	}

}

func TestGetMysqlUrlFromConfig(t *testing.T) {

	dpConfig := map[string]string{
		"database.user": "deskpro",
		"database.password": "deskpro",
		"database.host": "localhost",
		"database.dbname": "deskpro",
	}

	actualmUrl := GetMysqlUrlFromConfig(dpConfig, "database")
	expectedmUrl := *(&url.URL{
		Scheme:   "mysql",
		User:     url.UserPassword("deskpro", "deskpro"),
		Host:     "localhost",
		Path:     "/deskpro",
		RawPath:  "",
		RawQuery: "",
	})

	if !reflect.DeepEqual(actualmUrl, expectedmUrl) {
		t.Error("Expected and actual configs are not equal")
	}

}
