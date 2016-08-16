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
RPC

An RPC Client and Server Codec is implemented, so that msgpack can be used
with the standard net/rpc package. It supports both a basic net/rpc serialization,
and the custom format defined at http://wiki.msgpack.org/display/MSGPACK/RPC+specification

*/
package msgpack

import (
	"fmt"
	"strings"
	"net/rpc"
	"io"
)

type rpcCodec struct {
	rwc       io.ReadWriteCloser
	dec       *Decoder
	enc       *Encoder
}

type basicRpcCodec struct {
	rpcCodec
}

type customRpcCodec struct {
	rpcCodec
}

func newRPCCodec(conn io.ReadWriteCloser, opts DecoderContainerResolver) (rpcCodec) {
	return rpcCodec{
		rwc: conn,
		dec: NewDecoder(conn, opts),
		enc: NewEncoder(conn),
	}
}

// NewRPCClientCodec uses basic msgpack serialization for rpc communication from client side.
// 
// Sample Usage:
//   conn, err = net.Dial("tcp", "localhost:5555")
//   codec, err := msgpack.NewRPCClientCodec(conn, nil)
//   client := rpc.NewClientWithCodec(codec)
//   ... (see rpc package for how to use an rpc client)
func NewRPCClientCodec(conn io.ReadWriteCloser, opts DecoderContainerResolver) (rpc.ClientCodec) {
	return &basicRpcCodec{ newRPCCodec(conn, opts) }
}

// NewRPCServerCodec uses basic msgpack serialization for rpc communication from the server side.
func NewRPCServerCodec(conn io.ReadWriteCloser, opts DecoderContainerResolver) (rpc.ServerCodec) {
	return &basicRpcCodec{ newRPCCodec(conn, opts) }
}

// NewCustomRPCClientCodec uses msgpack serialization for rpc communication from client side, 
// but uses a custom protocol defined at http://wiki.msgpack.org/display/MSGPACK/RPC+specification
func NewCustomRPCClientCodec(conn io.ReadWriteCloser, opts DecoderContainerResolver) (rpc.ClientCodec) {
	return &customRpcCodec{ newRPCCodec(conn, opts) }
}
	
// NewCustomRPCServerCodec uses msgpack serialization for rpc communication from server side, 
// but uses a custom protocol defined at http://wiki.msgpack.org/display/MSGPACK/RPC+specification
func NewCustomRPCServerCodec(conn io.ReadWriteCloser, opts DecoderContainerResolver) (rpc.ServerCodec) {
	return &customRpcCodec{ newRPCCodec(conn, opts) }
}
	
// /////////////// RPC Codec Shared Methods ///////////////////
func (c *rpcCodec) write(objs ...interface{}) (err error) {
	for _, obj := range objs {
		if err = c.enc.Encode(obj); err != nil {
			return
		}
	}
	return
}

func (c *rpcCodec) read(objs ...interface{}) (err error) {
	for _, obj := range objs {
		if err = c.dec.Decode(obj); err != nil {
			return
		}
	}
	return
}

// maybeEOF is used to possibly return EOF for functions (e.g. ReadXXXHeader) that
// should return EOF if underlying connection was closed.
// This is important because rpc uses goroutines on clients (to support sync and async models)
// and on the server. Consequently, for calling Client.Close to work for example, the 
// ReadRequestHeader and ReadResponseHeader methods should return EOF if underlying network 
// connection was closed (e.g. by Client.Close). 
// 
// It's a best effort, as there's no general error returned for Using Closed Network Connection.
func (c *rpcCodec) maybeEOF(err error) (errx error) {
	if err == nil {
		return nil
	}
	// defer func() { fmt.Printf("maybeEOF: orig: %T, %v, returning: %T, %v\n", err, err, errx, errx) }()
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return io.EOF
	} 
	errstr := err.Error()
	if strings.HasSuffix(errstr, "use of closed network connection") {
		return io.EOF
	}
	// switch nerr := err.(type) {
	// case *net.OpError:
	// 	println(" ***** *net.OpError ***** ", nerr.Err.Error())
	// 	if nerr.Err.Error() == "use of closed network connection" {
	// 		return io.EOF
	// 	}
	// }
	return err
}

func (c *rpcCodec) Close() error {
	// fmt.Printf("Calling rpcCodec.Close: %v\n----------------------\n", string(debug.Stack()))
	return c.rwc.Close()
	
}

func (c *rpcCodec) ReadResponseBody(body interface{}) error {
	return c.dec.Decode(body)
}

// /////////////// Basic RPC Codec ///////////////////
func (c *basicRpcCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	return c.write(r, body)
}

func (c *basicRpcCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	return c.write(r, body)
}

func (c *basicRpcCodec) ReadRequestBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *basicRpcCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.maybeEOF(c.dec.Decode(r))
}

func (c *basicRpcCodec) ReadRequestHeader(r *rpc.Request) error {
	return c.maybeEOF(c.dec.Decode(r))
}

// /////////////// Custom RPC Codec ///////////////////
func (c *customRpcCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	return c.writeCustomBody(0, r.Seq, r.ServiceMethod, body)
}

func (c *customRpcCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	return c.writeCustomBody(1, r.Seq, r.Error, body)
}

func (c *customRpcCodec) ReadRequestBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *customRpcCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.maybeEOF(c.parseCustomHeader(1, &r.Seq, &r.Error))
}

func (c *customRpcCodec) ReadRequestHeader(r *rpc.Request) error {
	return c.maybeEOF(c.parseCustomHeader(0, &r.Seq, &r.ServiceMethod))
}

func (c *customRpcCodec) parseCustomHeader(expectTypeByte byte, msgid *uint64, methodOrError *string) (err error) {

	// We read the response header by hand 
	// so that the body can be decoded on its own from the stream at a later time.

	bs := make([]byte, 1)
	n, err := c.rwc.Read(bs)
	if err != nil {
		return 
	}
	if n != 1 {
		err = fmt.Errorf("Couldn't read array descriptor: No bytes read")
		return
	}
	const fia byte = 0x94 //four item array descriptor value
	if bs[0] != fia {
		err = fmt.Errorf("Unexpected value for array descriptor: Expecting %v. Received %v", fia, bs[0])
		return
	}
	var b byte
	if err = c.read(&b, msgid, methodOrError); err != nil {
		return
	}
	if b != expectTypeByte {
		err = fmt.Errorf("Unexpected byte descriptor in header. Expecting %v. Received %v", expectTypeByte, b)
		return
	}
	return
}

func (c *customRpcCodec) writeCustomBody(typeByte byte, msgid uint64, methodOrError string, body interface{}) (err error) {
	var moe interface{} = methodOrError
	// response needs nil error (not ""), and only one of error or body can be nil
	if typeByte == 1 {
		if methodOrError == "" {
			moe = nil
		}
		if moe != nil && body != nil {
			body = nil
		}
	}
	r2 := []interface{}{ typeByte, uint32(msgid), moe, body }
	return c.enc.Encode(r2)
}

