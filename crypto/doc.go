/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
/*
   This encryption scheme was originally used in the GRR communication
   protocol.

   It is designed for the following goals:

1. The protocol can be bootstrapped with zero knowledge - there is no
   need to store previous session keys (symmetric keys). If the
   session key for each packet is not known then it can always be
   possible to recalculate it.

2. Once the session key is derived, then it may be cached. Caching the
   session key saves the end point from performing expensive RSA
   operations. This is purely an optimization.

3. The cipher object contains the session key as well as the hmac
   key. The cipher proto is encrypted using the receiver public key so
   only the receiver may decrypt it.

4. To verify the authenticity of the cipher object, one must decrypt
   it, extract the session key and use that to decrypt the
   encrypted_cipher_metadata field. That field contains the RSA for
   the cipher object (signed using the sender's private key). Note
   that since the cipher does not change throughout the session,
   neither does the encrypted_cipher_metadata and so both can be
   cached (and signature verification is not needed if the cipher blob
   was seen previously.

5. Integrity of each packet's payload is assured through a HMAC. The
   hmac key is constant throughout the session and it is specified in
   the cipher object. Note we check the hmac before anything else to
   reject malformed packets earlier and save some cycles.

*/
package crypto
