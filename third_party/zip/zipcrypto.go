package zip

import (
	"hash/crc32"
	"io"
)

type ZipCrypto struct {
	password []byte
	Keys     [3]uint32
}

func NewZipCrypto(passphrase []byte) *ZipCrypto {
	z := &ZipCrypto{}
	z.password = passphrase
	z.init()
	return z
}

func (z *ZipCrypto) init() {
	z.Keys[0] = 0x12345678
	z.Keys[1] = 0x23456789
	z.Keys[2] = 0x34567890

	for i := 0; i < len(z.password); i++ {
		z.updateKeys(z.password[i])
	}
}

func (z *ZipCrypto) updateKeys(byteValue byte) {
	z.Keys[0] = crc32update(z.Keys[0], byteValue)
	z.Keys[1] += z.Keys[0] & 0xff
	z.Keys[1] = z.Keys[1]*134775813 + 1
	z.Keys[2] = crc32update(z.Keys[2], (byte)(z.Keys[1]>>24))
}

func (z *ZipCrypto) magicByte() byte {
	var t uint32 = z.Keys[2] | 2
	return byte((t * (t ^ 1)) >> 8)
}

func (z *ZipCrypto) Encrypt(data []byte) []byte {
	length := len(data)
	chiper := make([]byte, length)
	for i := 0; i < length; i++ {
		v := data[i]
		chiper[i] = v ^ z.magicByte()
		z.updateKeys(v)
	}
	return chiper
}

func (z *ZipCrypto) Decrypt(chiper []byte) []byte {
	length := len(chiper)
	plain := make([]byte, length)
	for i, c := range chiper {
		v := c ^ z.magicByte()
		z.updateKeys(v)
		plain[i] = v
	}
	return plain
}

type ZipCryptoReader struct {
	ZipCrypto
	upstream io.Reader
}

func NewZipCryptoReader(upstream io.Reader, passphrase []byte) *ZipCryptoReader {
	z := &ZipCryptoReader{
		ZipCrypto: ZipCrypto{
			password: passphrase,
		},
		upstream: upstream,
	}
	z.init()

	// skip first 12 bytes
	buf := make([]byte, 12)
	_, _ = z.Read(buf)

	return z
}

func (z *ZipCryptoReader) Read(p []byte) (n int, err error) {
	n, err = z.upstream.Read(p)
	for i := 0; i < n; i++ {
		c := p[i]
		v := c ^ z.magicByte()
		z.updateKeys(v)
		p[i] = v
	}
	return n, err
}

func crc32update(pCrc32 uint32, bval byte) uint32 {
	return crc32.IEEETable[(pCrc32^uint32(bval))&0xff] ^ (pCrc32 >> 8)
}

func ZipCryptoDecryptor(r *io.SectionReader, password []byte) (io.Reader, error) {
	z := NewZipCryptoReader(r, password)
	return z, nil
}

type zipCryptoWriter struct {
	w     io.Writer
	z     *ZipCrypto
	first bool
	fw    *fileWriter
}

func (z *zipCryptoWriter) Write(p []byte) (n int, err error) {
	err = nil
	if z.first {
		z.first = false
		header := []byte{0xF8, 0x53, 0xCF, 0x05, 0x2D, 0xDD, 0xAD, 0xC8, 0x66, 0x3F, 0x8C, 0xAC}
		header = z.z.Encrypt(header)

		crc := z.fw.ModifiedTime
		header[10] = byte(crc)
		header[11] = byte(crc >> 8)

		z.z.init()
		_, _ = z.w.Write(z.z.Encrypt(header))
		n += 12
	}
	_, _ = z.w.Write(z.z.Encrypt(p))
	return
}

func ZipCryptoEncryptor(i io.Writer, pass passwordFn, fw *fileWriter) (io.Writer, error) {
	z := NewZipCrypto(pass())
	zc := &zipCryptoWriter{i, z, true, fw}
	return zc, nil
}
