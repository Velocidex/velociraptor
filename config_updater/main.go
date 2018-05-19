package main

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"os"
	"regexp"
)

var (
	repack        = kingpin.Command("repack", "Repack a binary.")
	repack_config = repack.Arg(
		"config_file", "The config file to repack into the binary.").
		Required().String()

	repack_binary = repack.Arg("binary", "The binary to repack.").Required().String()

	extract        = kingpin.Command("extract", "Extract configuration")
	extract_binary = extract.Arg("binary", "The binary to repack.").Required().String()
)

func RepackClient() error {
	binary, err := ioutil.ReadFile(*repack_binary)
	if err != nil {
		return err
	}

	config_file, err := ioutil.ReadFile(*repack_config)
	if err != nil {
		return err
	}

	start := []byte("# START CONFIGURATION")
	end := []byte("# END CONFIGURATION")
	offset := bytes.Index(binary, start)
	if offset == -1 {
		return errors.New("No magic start")
	}

	fmt.Printf("Read %v bytes", offset)

	new_binary := append(binary[:offset+len(start)], config_file...)
	new_binary = append(new_binary, end...)
	new_binary = append(new_binary,
		binary[offset+len(start)+len(config_file)+len(end):]...)

	err = ioutil.WriteFile(*repack_binary, new_binary, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func ExtractConfig() error {
	binary, err := ioutil.ReadFile(*extract_binary)
	if err != nil {
		return err
	}

	re := regexp.MustCompile("(?sm)# START C[O]NFIGURATION(.+)?# END CONFIGURATION")

	result := re.FindStringSubmatch(string(binary))
	if len(result) > 1 {
		fmt.Printf("%v", result[1])
		return nil
	}

	return errors.New("Enable to find embedded config")

}

func main() {
	switch kingpin.Parse() {
	case "repack":
		err := RepackClient()
		if err != nil {
			kingpin.FatalIfError(err, "Can not repack client")
		}

	case "extract":
		err := ExtractConfig()
		if err != nil {
			kingpin.FatalIfError(err, "Can not extract client")
		}
	}
}
