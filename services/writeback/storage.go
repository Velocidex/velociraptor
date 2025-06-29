package writeback

import (
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"runtime"

	"github.com/Velocidex/yaml/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type WritebackStorer interface {
	// Write the wb into the L1 storage
	WriteL1(wb *config_proto.Writeback) error

	// Write the wb into the L2 storage
	WriteL2(wb *config_proto.Writeback) error

	// Load the wb from the L1 and merge with the L2 store
	Load() *config_proto.Writeback
}

type FileWritebackStore struct {
	config_obj  *config_proto.Config
	location    string
	l2_location string
}

func (self *FileWritebackStore) writeToFile(
	wb *config_proto.Writeback, location string) error {
	bytes, err := yaml.Marshal(wb)
	if err != nil {
		return fmt.Errorf("Writeback WriteFile to %v: %w", location, err)
	}

	err = ioutil.WriteFile(location, bytes, 0600)
	if err != nil {
		return fmt.Errorf("Writeback WriteFile to %v: %w", location, err)
	}
	return nil
}

func (self *FileWritebackStore) readFromFile(
	location string) (*config_proto.Writeback, error) {

	writeback := &config_proto.Writeback{}
	data, err := ioutil.ReadFile(location)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, writeback)
	return writeback, err
}

// When we have 2 writebacks, the Level1 writeback only has a few
// fields.
func (self *FileWritebackStore) WriteL1(wb *config_proto.Writeback) error {
	var to_write *config_proto.Writeback
	if self.location == self.l2_location {
		to_write = wb
	} else {
		to_write = &config_proto.Writeback{
			InstallTime: wb.InstallTime,
			PrivateKey:  wb.PrivateKey,
			ClientId:    wb.ClientId,
		}
	}

	return self.writeToFile(
		to_write, self.location)
}

// The level2 file contains all the fields - as a backup to the l1
// file.
func (self *FileWritebackStore) WriteL2(wb *config_proto.Writeback) error {
	return self.writeToFile(wb, self.l2_location)
}

func (self *FileWritebackStore) Load() *config_proto.Writeback {
	wb, l2_err := self.readFromFile(self.l2_location)
	if l2_err != nil {
		wb = &config_proto.Writeback{}
	}

	logger := logging.GetLogger(self.config_obj, &logging.ClientComponent)
	l1_wb, err := self.readFromFile(self.location)
	if err == nil {
		wb.InstallTime = l1_wb.InstallTime
		wb.PrivateKey = l1_wb.PrivateKey
		wb.ClientId = l1_wb.ClientId
		logger.Info("Writeback Manager: Loading config from writeback (%v)", self.location)
	} else {
		if l2_err == nil {
			logger.Info("Writeback Manager: Unable to read primary writeback (%v) - will reset from secondary %v",
				err, self.l2_location)

			// Restore the L1 file from the backup
			err = self.WriteL1(wb)
			if err != nil {
				logger.Error("Writeback Manager:  WriteL1: %v", err)
			}

		} else {
			logger.Info("Writeback Manager: Unable to read writeback (%v) - will reset", err)
		}
	}

	if wb.InstallTime == 0 {
		wb.InstallTime = uint64(utils.GetTime().Now().Unix())
		err := self.WriteL1(wb)
		if err != nil {
			logger.Error("Writeback Manager:  WriteL1: %v", err)
		}
	}

	return wb
}

// Return the location of the writeback file.
func WritebackLocation(
	config_obj *config_proto.Config) (string, error) {
	if config_obj == nil || config_obj.Client == nil {
		return "", errors.New("Client not configured")
	}

	result := ""

	nonce := config_obj.Client.Nonce

	switch runtime.GOOS {
	case "darwin":
		result = expandOrgId(config_obj.Client.WritebackDarwin, nonce)

	case "linux":
		result = expandOrgId(config_obj.Client.WritebackLinux, nonce)

	case "windows":
		result = expandOrgId(config_obj.Client.WritebackWindows, nonce)

	default:
		result = expandOrgId(config_obj.Client.WritebackLinux, nonce)
	}

	if result == "" {
		return "", errors.New("Client Writeback is not configured")
	}

	return result, nil
}

var (
	org_id_regex = regexp.MustCompile("\\$NONCE|%NONCE%")
)

func expandOrgId(path, nonce string) string {
	path = org_id_regex.ReplaceAllString(path, utils.SanitizeString(nonce))
	return utils.ExpandEnv(path)
}
