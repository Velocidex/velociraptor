package raw_registry

import "www.velocidex.com/golang/regparser"

// A helper method to open a key by path.
func OpenKeyComponents(
	self *regparser.Registry, components []string) *regparser.CM_KEY_NODE {

	root_cell := self.Profile.HCELL(self.Reader,
		0x1000+int64(self.BaseBlock.RootCell()))

	nk := root_cell.KeyNode()
	if nk == nil {
		return nil
	}

subkey_match:
	for _, component := range components {
		if component == "" {
			continue
		}

		for _, subkey := range nk.Subkeys() {
			if subkey.Name() == component {
				nk = subkey
				continue subkey_match
			}
		}

		// If we get here we could not find the key:
		return nil
	}

	return nk
}
