
/*
go-msgpack - Msgpack library for Go. Provides pack/unpack and net/rpc support.
https://github.com/ugorji/go-msgpack

Copyright (c) 2012, Ugorji Nwoke.
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice,
  this list of conditions and the following disclaimer.
* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.
* Neither the name of the author nor the names of its contributors may be used
  to endorse or promote products derived from this software
  without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

/*
MsgPack library for Go (DEPRECATED - Replacement: go get github.com/ugorji/go/codec)

  THIS LIBRARY IS DEPRECATED (May 29, 2013)
  
  Please use github.com/ugorji/go/codec
  which is significantly faster, cleaner, more correct and more complete.
  See [https://github.com/ugorji/go/tree/master/codec#readme]
  
  A complete redesign was done which also accomodates multiple codec formats.
  It thus became necessary to create a new repository with a different name. 
  
  I hope to retire this repository anytime from July 1, 2013.
  
  A log message will be printed out at runtime encouraging users to upgrade.

Implements:
  http://wiki.msgpack.org/display/MSGPACK/Format+specification

It provides features similar to encoding packages in the standard library (ie json, xml, gob, etc).

Supports:
  - Standard Marshal/Unmarshal interface.
  - Standard field renaming via tags
  - Encoding from any value (struct, slice, map, primitives, pointers, interface{}, etc)
  - Decoding into pointer to any non-nil value (struct, slice, map, int, float32, bool, string, etc)
  - Decoding into a nil interface{} 
  - Handles time.Time transparently 
  - Provides a Server and Client Codec so msgpack can be used as communication protocol for net/rpc.
    Also includes an option for msgpack-rpc: http://wiki.msgpack.org/display/MSGPACK/RPC+specification

Usage

  dec = msgpack.NewDecoder(r, nil)
  err = dec.Decode(&v) 
  
  enc = msgpack.NewEncoder(w)
  err = enc.Encode(v) 
  
  //methods below are convenience methods over functions above.
  data, err = msgpack.Marshal(v) 
  err = msgpack.Unmarshal(data, &v, nil)
  
  //RPC Server
  conn, err := listener.Accept()
  rpcCodec := msgpack.NewRPCServerCodec(conn, nil)
  rpc.ServeCodec(rpcCodec)

  //RPC Communication (client side)
  conn, err = net.Dial("tcp", "localhost:5555")  
  rpcCodec := msgpack.NewRPCClientCodec(conn, nil)  
  client := rpc.NewClientWithCodec(rpcCodec)  
 
*/
package msgpack

import golog "log"
func init() {
	//printout deprecation notice
	golog.Print(`
************************************************ 
package github.com/ugorji/go-msgpack has been deprecated (05/29/2013). 
It will be retired anytime from July 1, 2013.
Please update to faster and much much better github.com/ugorji/go/codec.
See https://github.com/ugorji/go/tree/master/codec#readme for more information.
************************************************ 
`)
}
