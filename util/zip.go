package util

import (
	"io"

	"github.com/alexmullins/zip"
)

func ZipCreate(writer *zip.Writer, name string, secret string) (io.Writer, error) {
	if secret == "" {
		return writer.Create(name)
	} else {
		return writer.Encrypt(name, secret)
	}
}
