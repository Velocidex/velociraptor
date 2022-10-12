# Go `archive/zip` plus encryption support

[![GoDoc](https://godoc.org/github.com/jarnoan/zip?status.svg)](https://godoc.org/github.com/jarnoan/zip)
[![Go Report Card](https://goreportcard.com/badge/github.com/jarnoan/zip)](https://goreportcard.com/report/github.com/jarnoan/zip)

This is a fork of the `archive/zip` package from the Go standard
library which adds support for both the legacy
(insecure) ZIP encryption scheme and for newer AES-based encryption
schemes introduced with WinZip. It is based on Go 1.14.

This is based on work by [Alex Mullins](https://github.com/alexmullins/zip),
[Yakub Kristianto](https://github.com/yeka/zip) and [Hilko Bengen](https://github.com/hillu/go-archive-zip-crypto). 
The forward-port was done to introduce bugfixes and enhancements, such as missing support for large
(>= 4GB) ZIP files like those distributed by [VirusShare](https://virusshare.com/).

This fork improves the earlier ones by allowing decryption of large files
without reading the whole file in memory at once.
Encryption could be done similarly but I didn't have need for that.
