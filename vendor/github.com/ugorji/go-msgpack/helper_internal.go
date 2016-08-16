
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

// All non-std package dependencies live in this file,
// so porting to different environment is easy (just update functions).

import (
	"testing"
	"reflect"
	"fmt"
	"errors"
)

var (
	raisePanicAfterRecover = false
	showLog = true
	debugging = false
	testLogToT = true
	failNowOnFail = true
)

func checkErrT(t *testing.T, err error) {
	if err != nil {
		logT(t, err.Error())
		failT(t)
	}
}

func checkEqualT(t *testing.T, v1 interface{}, v2 interface{}) {
	if !reflect.DeepEqual(v1, v2) { 
		logT(t, "Do not match: v1: %v, v2: %v", v1, v2)
		failT(t)
	}
}

func panicToErrT(panicVal interface{}, err *error) {
	switch xerr := panicVal.(type) {
	case error:
		*err = xerr
	case string:
		*err = errors.New(xerr)
	default:
		*err = fmt.Errorf("%v", panicVal)
	}
	if raisePanicAfterRecover {
		panic(panicVal)
	}
	return
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func approxDataSize(rv reflect.Value) (sum int) {
	switch rk := rv.Kind(); rk {
	case reflect.Invalid:
	case reflect.Ptr, reflect.Interface:
		sum += int(rv.Type().Size())
		sum += approxDataSize(rv.Elem())
	case reflect.Slice:
		sum += int(rv.Type().Size())
		for j := 0; j < rv.Len(); j++ {
			sum += approxDataSize(rv.Index(j))
		}
	case reflect.String:
		sum += int(rv.Type().Size()) 
		sum += rv.Len()
	case reflect.Map:
		sum += int(rv.Type().Size()) 
		for _, mk := range rv.MapKeys() {
			sum += approxDataSize(mk)
			sum += approxDataSize(rv.MapIndex(mk))
		}
	case reflect.Struct:
		//struct size already includes the full data size.
		//sum += int(rv.Type().Size())
		for j := 0; j < rv.NumField(); j++ {
			sum += approxDataSize(rv.Field(j))
		}	
	default:
		//pure value types
		sum += int(rv.Type().Size())
	}
	return
}

func logT(x interface{}, format string, args ...interface{}) {
	if t, ok := x.(*testing.T); ok && t != nil && testLogToT {
		t.Logf(format, args...)	
	} else if b, ok := x.(*testing.B); ok && b != nil && testLogToT {
		b.Logf(format, args...)
	} else {
		log(format, args...)
	}
}

func failT(t *testing.T) {
	if failNowOnFail {
		t.FailNow()
	} else {
		t.Fail()
	}
}

func log(format string, args ...interface{}) {
	if showLog || debugging {
		if len(format) == 0 || format[len(format)-1] != '\n' {
			format = format + "\n"
		}
		fmt.Printf(format, args...)
	}
}

