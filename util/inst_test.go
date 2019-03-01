package util

import (
	"fmt"
	"testing"
)

func TestCheckDpDir(t *testing.T) {
	err := CheckDpDir("../test_mocks/dp_dir")

	if err != nil{
		t.Error(fmt.Sprintf("%s", err))
	}
}