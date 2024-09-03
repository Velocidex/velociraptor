package client

import (
	"testing"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestClientKeyLRU(t *testing.T) {
	lru := NewCipherLRU(2)

	client_id := "Client1"

	var cipher *_Cipher
	var pres bool

	inbound1 := []byte{1}
	outbound1 := []byte{2}

	// Do we have an outbound cipher? Not currently
	cipher, pres = lru.GetOutboundCipher(client_id)
	assert.False(t, pres)

	// Add the outbound cipher now.
	lru.Set(client_id, nil, &_Cipher{
		source:           client_id,
		encrypted_cipher: outbound1,
	})

	// Check for outbound cipher LRU hit   should exist now
	cipher, pres = lru.GetOutboundCipher(client_id)
	assert.True(t, pres)
	assert.Equal(t, cipher.encrypted_cipher, outbound1)

	// Check for inbound cipher - none set yet.
	cipher, pres = lru.GetByInboundCipher(inbound1)
	assert.False(t, pres)

	// Set the inbound cipher
	lru.Set(client_id, &_Cipher{
		source:           client_id,
		encrypted_cipher: inbound1,
	}, nil)

	// Check for inbound cipher - should be found now.
	cipher, pres = lru.GetByInboundCipher(inbound1)
	assert.True(t, pres)
	assert.Equal(t, cipher.encrypted_cipher, inbound1)
	assert.Equal(t, cipher.source, client_id)

	// Check for inbound cipher again - should still be cached
	cipher, pres = lru.GetByInboundCipher(inbound1)
	assert.True(t, pres)

	// Now change the ciphers around and check the LRU is updating
	// properly.
	for i := byte(0); i < 10; i++ {
		inbound2 := []byte{i}

		// Set the inbound cipher
		lru.Set(client_id, &_Cipher{
			source:           client_id,
			encrypted_cipher: inbound2,
		}, nil)

		// Is it properly updating?
		cipher, pres = lru.GetByInboundCipher(inbound2)
		assert.True(t, pres)
		assert.Equal(t, cipher.encrypted_cipher, inbound2)

		// Update outbound ciphers now.
		outbound2 := []byte{i + 20}
		lru.Set(client_id, nil, &_Cipher{
			source:           client_id,
			encrypted_cipher: outbound2,
		})

		// Is it properly updating?
		cipher, pres = lru.GetOutboundCipher(client_id)
		assert.True(t, pres)
		assert.Equal(t, cipher.encrypted_cipher, outbound2)
	}

	// Make sure we dont leak
	assert.Equal(t, 1, len(lru.by_source))
	assert.Equal(t, 1, len(lru.by_inbound_cipher))
	assert.Equal(t, int64(1), lru.size)
}
