package crypto

import (
	"errors"
	"math/big"

	"www.velocidex.com/golang/velociraptor/json"
)

// The default bigint marshaller uses a long integer but this may not
// be properly parsed - to be safe we just emit a string.
func MarshalBigInt(v interface{}, opts *json.EncOpts) ([]byte, error) {
	bint, ok := v.(*big.Int)
	if !ok {
		return nil, errors.New("Not a bigint")
	}

	encoded, err := bint.MarshalJSON()
	if err != nil {
		return nil, err
	}

	res := make([]byte, len(encoded)+2)
	res[0] = '"'
	res[len(res)-1] = '"'
	copy(res[1:], encoded)
	return res, nil
}

func init() {
	json.RegisterCustomEncoder(&big.Int{}, MarshalBigInt)
}
