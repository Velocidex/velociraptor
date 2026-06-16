package main_test

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"strings"

	"github.com/xor-gate/ar"
)

type debFeatures struct {
	ConfigFile string
	Binary     string
}

func extractDebFeatures(fd io.Reader) (*debFeatures, error) {
	res := &debFeatures{}

	ar_obj := ar.NewReader(fd)

	for {
		header, err := ar_obj.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return res, err
		}

		if strings.HasSuffix(header.Name, ".tar.gz") {
			gzf, err := gzip.NewReader(ar_obj)
			if err != nil {
				return res, err
			}

			tarReader := tar.NewReader(gzf)
			for {
				header, err := tarReader.Next()
				if err == io.EOF {
					break
				}

				if err != nil {
					return res, err
				}

				switch header.Name {
				case "etc/velociraptor/server.config.yaml":
					res.ConfigFile = readAll(tarReader)
				case "usr/local/bin/velociraptor":
					res.Binary = readAll(tarReader)
				}
				//fmt.Printf("Filename %v\n", header.Name)
			}
		}
		//fmt.Printf("Filename %v\n", header.Name)
	}
	return res, nil
}

func readAll(fd io.Reader) string {
	data, err := io.ReadAll(fd)
	if err != nil {
		return ""
	}

	return string(data)
}
