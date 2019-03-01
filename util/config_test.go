package util

import (
	"fmt"
	"testing"
	"reflect"
)

func TestConfig_Paths(t *testing.T) {
	config := Config{}
	config.SetPhpPath("/path/to/php").SetDpPath("/path/to/dp")

	expected := "/path/to/php"
	actual := config.PhpPath()

	if actual != expected {
		t.Error(fmt.Sprintf("Expected {%s} was {%s}", expected, actual))
	}

	expected = "/path/to/dp"
	actual = config.DpPath()

	if actual != expected {
		t.Error(fmt.Sprintf("Expected {%s} was {%s}", expected, actual))
	}
}

func TestConfig_GetDeskproConfig(t *testing.T) {

	config := Config{"", "", map[string]string{"test_value": "value"}}

	expectedConfig := map[string]string{"test_value": "value"}
	actualConfig, err := config.GetDeskproConfig()

	if err != nil{
		t.Error(fmt.Sprintf("%s", err))
	}

	if !reflect.DeepEqual(expectedConfig, actualConfig) {
		t.Error("Expected and actual configs are not equal")
	}
}
