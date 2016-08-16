
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

package msgpack

// Code here is organized as follows:
// Exported methods are not called internally. They are just facades.
//   Marshal calls Encode 
//   Encode calls EncodeValue 
//   EncodeValue calls encodeValue 
// encodeValue and all other unexported functions use panics (not errors)
//    and may call other unexported functions (which use panics).
// 

import (
	"io"
	"bytes"
	"reflect"
	"math"
	"time"
	"encoding/binary"
)

var (
	// Some tagging information for error messages.
	msgTagEnc = "msgpack.encoder"
) 

// An Encoder writes an object to an output stream in the msgpack format.
type Encoder struct {
	w io.Writer
	x [16]byte        //temp byte array re-used internally for efficiency
	t1, t2, t3, t31, t5, t51, t9, t91 []byte // use these, so no need to constantly re-slice
}

// NewDecoder returns an Encoder for encoding an object.
func NewEncoder(w io.Writer) (e *Encoder) {	
	e = &Encoder{w:w}
	e.t1, e.t2, e.t3, e.t31, e.t5, e.t51, e.t9, e.t91 = 
		e.x[:1], e.x[:2], e.x[:3], e.x[1:3], e.x[:5], e.x[1:5], e.x[:9], e.x[1:9]
	return
}

// Encode writes an object into a stream in the MsgPack format.
// 
// time.Time is handled transparently, by (en)decoding (to)from a 
// []int64{Seconds since Epoch, Nanoseconds offset}.
// 
// Struct values encode as maps. Each exported struct field is encoded unless:
//    - the field's tag is "-", or
//    - the field is empty and its tag specifies the "omitempty" option.
//
// The empty values are false, 0, any nil pointer or interface value, 
// and any array, slice, map, or string of length zero. 
// 
// Anonymous fields are encoded inline if no msgpack tag is present.
// Else they are encoded as regular fields.
// 
// The object's default key string is the struct field name but can be 
// specified in the struct field's tag value. 
// The "msgpack" key in struct field's tag value is the key name, 
// followed by an optional comma and options. 
// 
// To set an option on all fields (e.g. omitempty on all fields), you 
// can create a field called _struct, and set flags on it.
// 
// Examples:
//    
//      type MyStruct struct {
//          _struct bool    `msgpack:",omitempty"`   //set omitempty for every field
//          Field1 string   `msgpack:"-"`            //skip this field
//          Field2 int      `msgpack:"myName"`       //Use key "myName" in encode stream
//          Field3 int32    `msgpack:",omitempty"`   //use key "Field3". Omit if empty.
//          Field4 bool     `msgpack:"f4,omitempty"` //use key "f4". Omit if empty.
//          ...
//      }
//    
func (e *Encoder) Encode(v interface{}) (err error) {
	return e.EncodeValue(reflectValue(v))
}

// EncodeValue encodes a reflect.Value.
func (e *Encoder) EncodeValue(rv reflect.Value) (err error) {
	defer panicToErr(&err) 
	e.encodeValue(rv)
	return
}

func (e *Encoder) encode(v interface{}) {
	e.encodeValue(reflectValue(v))
}

func (e *Encoder) encodeValue(rv reflect.Value) {
	//log("++ enter encode rv: %v, %v", rv, rv.Interface())
	//defer func() {
	//	log("++  exit encode rv: %v, %v", rv, rv.Interface())
	//}()
	
	// Tested with a type assertion for all common types first, but this increased encoding time
	// sometimes by up to 20% (weird). So just use the reflect.Kind switch alone.
	
	// ensure more common cases appear early in switch.
	switch rk := rv.Kind(); rk {
	case reflect.Bool:
		e.encBool(rv.Bool())
	case reflect.String:
		e.encString(rv.String())
	case reflect.Int, reflect.Int8, reflect.Int64, reflect.Int32, reflect.Int16:
		e.encInt(rv.Int())
	case reflect.Uint8, reflect.Uint64, reflect.Uint, reflect.Uint32, reflect.Uint16:
		e.encUint(rv.Uint())
	case reflect.Float64:
		e.t9[0] = 0xcb
		binary.BigEndian.PutUint64(e.t91, math.Float64bits(rv.Float()))
		e.writeb(9, e.t9)
	case reflect.Float32:
		e.t5[0] = 0xca
		binary.BigEndian.PutUint32(e.t51, math.Float32bits(float32(rv.Float())))
		e.writeb(5, e.t5)
	case reflect.Slice:
		if rv.IsNil() {
			e.encNil()
			break
		} 
		l := rv.Len()
		if rv.Type() == byteSliceTyp {
			e.writeContainerLen(ContainerRawBytes, l)
			if l > 0 {
				e.writeb(l, rv.Bytes())
			}
			break
		}
		e.writeContainerLen(ContainerList, l)
		for j := 0; j < l; j++ {
			e.encode(rv.Index(j))
		}
	case reflect.Array:
		l := rv.Len()
		// this should not happen (a 0-elem array makes no sense) ... but just in case
		if l == 0 {
			e.writeContainerLen(ContainerList, l)
			break
		}
		// log("---- %v", rv.Type())
		// if rv.Type().Elem().Kind == reflect.Uint8 { // surprisingly expensive (check 1st value instead)
		if rv.Index(0).Kind() == reflect.Uint8 {
			e.writeContainerLen(ContainerRawBytes, l)
			e.writeb(l, rv.Slice(0, l).Bytes())
			break
		}
		e.writeContainerLen(ContainerList, l)
		for j := 0; j < l; j++ {
			e.encode(rv.Index(j))
		}
	case reflect.Map:
		if rv.IsNil() {
			e.encNil()
			break
		}
		e.writeContainerLen(ContainerMap, rv.Len())
		for _, mk := range rv.MapKeys() {
			e.encode(mk)
			e.encode(rv.MapIndex(mk))
		}
	case reflect.Struct:
		rt := rv.Type()
		//treat time.Time specially
		if rt == timeTyp {
			tt := rv.Interface().(time.Time)
			e.encode([2]int64{tt.Unix(), int64(tt.Nanosecond())})
			break
		}
		e.encodeStruct(rt, rv)
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			e.encNil()
			break
		}
		e.encodeValue(rv.Elem())
	case reflect.Invalid:
		e.encNil()
	default:
		e.err("Unsupported kind: %s, for: %#v", rk, rv)
	}
	return
}

func (e *Encoder) writeContainerLen(ct ContainerType, l int) {
	locutoff, b0, b1, b2 := getContainerByteDesc(ct)

	switch {
	case l < locutoff:
		e.t1[0] = (b0 | byte(l))
		e.writeb(1, e.t1)
	case l < 65536:
		e.t3[0] = b1
		binary.BigEndian.PutUint16(e.t31, uint16(l))
		e.writeb(3, e.t3)
	default:
		e.t5[0] = b2
		binary.BigEndian.PutUint32(e.t51, uint32(l))
		e.writeb(5, e.t5)
	}
}

func (e *Encoder) encNil() {
	e.t1[0] = 0xc0
	e.writeb(1, e.t1)
}

func (e *Encoder) encInt(i int64) {
	switch {
	case i < math.MinInt32 || i > math.MaxInt32:
		e.t9[0] = 0xd3
		binary.BigEndian.PutUint64(e.t91, uint64(i))
		e.writeb(9, e.t9)
	case i < math.MinInt16 || i > math.MaxInt16:
		e.t5[0] = 0xd2
		binary.BigEndian.PutUint32(e.t51, uint32(i))
		e.writeb(5, e.t5)
	case i < math.MinInt8 || i > math.MaxInt8:
		e.t3[0] = 0xd1
		binary.BigEndian.PutUint16(e.t31, uint16(i))
		e.writeb(3, e.t3)
	case i < -32:
		e.t2[0], e.t2[1] = 0xd0, byte(i)
		e.writeb(2, e.t2)
	case i >= -32 && i <= math.MaxInt8:
		e.t1[0] = byte(i)
		e.writeb(1, e.t1)
	default:
		e.err("encInt64: Unreachable block")
	}
}

func (e *Encoder) encUint(i uint64) {
	switch {
	case i <= math.MaxInt8:
		e.t1[0] = byte(i)
		e.writeb(1, e.t1)
	case i <= math.MaxUint8:
		e.t2[0], e.t2[1] = 0xcc, byte(i)
		e.writeb(2, e.t2)
	case i <= math.MaxUint16:
		e.t3[0] = 0xcd
		binary.BigEndian.PutUint16(e.t31, uint16(i))
		e.writeb(3, e.t3)
	case i <= math.MaxUint32:
		e.t5[0] = 0xce
		binary.BigEndian.PutUint32(e.t51, uint32(i))
		e.writeb(5, e.t5)
	default:
		e.t9[0] = 0xcf
		binary.BigEndian.PutUint64(e.t91, i)
		e.writeb(9, e.t9)
	}
}

func (e *Encoder) encBool(b bool) {
	if b {
		e.t1[0] = 0xc3
	} else {
		e.t1[0] = 0xc2
	}
	e.writeb(1, e.t1)
}

func (e *Encoder) encodeStruct(rt reflect.Type, rv reflect.Value) {
	sis := getStructFieldInfos(rt)
	// e.writeContainerLen(ContainerMap, len(sis.sis))
	// for _, si := range sis.sis {
	// 	e.encode(si.encNameBs)
	// 	e.encode(si.field(rv))
	// }
	// return
	
	encNames := make([][]byte, len(sis.sis))
	rvals := make([]reflect.Value, len(sis.sis))
	newlen := 0
	for _, si := range sis.sis {
		rval0 := si.field(rv)
		if si.omitEmpty && isEmptyValue(rval0) {
			continue
		}
		encNames[newlen] = si.encNameBs
		rvals[newlen] = rval0
		newlen++
	}
	
	e.writeContainerLen(ContainerMap, newlen)
	for j := 0; j < newlen; j++ {
		e.encode(encNames[j])
		e.encode(rvals[j])
	}
	
}

func (e *Encoder) encString(s string) {
	numbytes := len(s)
	e.writeContainerLen(ContainerRawBytes, numbytes)
	// e.encode([]byte(s)) // using io.WriteString is faster
	n, err := io.WriteString(e.w, s)
	if err != nil {
		e.err("Error: %v", err)
	}
	if n != numbytes {
		e.err("write: Incorrect num bytes written. Expecting: %v, Wrote: %v", numbytes, n)
	}
}

func (e *Encoder) writeb(numbytes int, bs []byte) {
	// no sanity checking. Assume callers pass valid arguments. It's pkg-private: we can control it.
	n, err := e.w.Write(bs)
	if err != nil {
		// propagage io.EOF upwards (it's special, and must be returned AS IS)
		if err == io.EOF {
			panic(err)
		} else {
			e.err("Error: %v", err)
		}
	}
	if n != numbytes {
		e.err("write: Incorrect num bytes written. Expecting: %v, Wrote: %v", numbytes, n)
	}
}

func (e *Encoder) err(format string, params ...interface{}) {
	doPanic(msgTagEnc, format, params)
}

// Marshal is a convenience function which encodes v to a stream of bytes. 
// It delegates to Encoder.Encode.
func Marshal(v interface{}) (b []byte, err error) {
	bs := new(bytes.Buffer)
	if err = NewEncoder(bs).Encode(v); err == nil {
		b = bs.Bytes()
	}
	return
}


