package vql

import (
	"fmt"
	"os"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type DeviceMapping struct {
	Source   string
	MapAs    string
	Type     string
	Accessor glob.FileSystemAccessor
}

type DeviceManager struct {
	// Maps a device to its corresponding DeviceMapping object. The keys
	// of the map are the device identifiers.
	mappings map[string]*DeviceMapping
}

func MakeNewDeviceManager(scope vfilter.Scope, remappings []*config_proto.RemappingConfig) *DeviceManager {
	// if the user requested device mappings, these replace the host's filesystem
	if remap_count := len(remappings); remap_count > 0 {
		mappings := make(map[string]*DeviceMapping)

		for i, mapping := range remappings {
			if mapping.Source == "" {
				scope.Log("remapping: `source' of entry %d must not be empty - ignoring entry", i)
				continue
			}

			if mapping.MapAs == "" {
				scope.Log("remapping: `map_as' of entry %d must not be empty - ignoring entry", i)
				continue
			}

			if mapping.Type == "" {
				scope.Log("remapping: `type' of entry %d must not be empty - ignoring entry", i)
				continue
			}

			if _, err := os.Stat(mapping.Source); err != nil {
				scope.Log("remapping: cannot stat source %s (%s) of entry %d - ignoring entry", mapping.Source, err, i)
				continue
			}

			deviceName := `\\.\` + mapping.MapAs

			if existing, pres := mappings[deviceName]; pres {
				scope.Log(`remapping: mapping "%s" -> "%s" of entry %d would overwrite the existing mapping "%s" -> "%s" - ignoring entry`, mapping.Source, deviceName, i, existing.Source, existing.MapAs)
				continue
			}

			scope.Log(`remapping: mapping "%s" -> "%s" (%s)`, mapping.Source, deviceName, mapping.Type)

			mappings[deviceName] = &DeviceMapping{
				Source: mapping.Source,
				MapAs:  deviceName,
				Type:   mapping.Type,
			}

		}

		return &DeviceManager{
			mappings: mappings,
		}
	}

	// "map" the host's filesystem at the expected location
	mapping := make(map[string]*DeviceMapping)
	mapping[`\\.\`] = &DeviceMapping{
		Source: "/",
		MapAs:  `\\.\`,
		Type:   "file",
	}

	return &DeviceManager{
		mappings: mapping,
	}
}

func (self DeviceManager) GetAccessor(device string, scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	mapping, pres := self.mappings[device]
	if !pres {
		return nil, fmt.Errorf("device %s not registered with device manager", device)

	}

	if mapping.Accessor != nil {
		return mapping.Accessor, nil
	}

	accessor, err := glob.GetAccessor(mapping.Type, scope)
	if err != nil {
		return nil, err
	}

	mapping.Accessor = accessor
	return accessor, nil
}

func (self DeviceManager) GetDevices() []string {
	var keys []string

	for key := range self.mappings {
		keys = append(keys, key)
	}

	return keys
}

func (self DeviceManager) GetDeviceSource(device string) (string, error) {
	if mapping, pres := self.mappings[device]; pres {
		return mapping.Source, nil
	}

	return "", fmt.Errorf("device %s not registered with device manager", device)
}
