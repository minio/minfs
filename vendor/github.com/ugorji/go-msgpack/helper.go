
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

import (
	"unicode"
	"unicode/utf8"
	"reflect"
	"sync"
	"strings"
	"fmt"
	"time"
)

type ContainerType byte

const (
	ContainerRawBytes = ContainerType('b')
	ContainerList = ContainerType('a')
	ContainerMap = ContainerType('m')
)

var (
	structInfoFieldName = "_struct"
	
	cachedStructFieldInfos = make(map[reflect.Type]*structFieldInfos, 4)
	cachedStructFieldInfosMutex sync.Mutex

	nilIntfSlice = []interface{}(nil)
	intfSliceTyp = reflect.TypeOf(nilIntfSlice)
	intfTyp = intfSliceTyp.Elem()
	byteSliceTyp = reflect.TypeOf([]byte(nil))
	timeTyp = reflect.TypeOf(time.Time{})
	mapStringIntfTyp = reflect.TypeOf(map[string]interface{}(nil))
	mapIntfIntfTyp = reflect.TypeOf(map[interface{}]interface{}(nil))
)

type structFieldInfo struct {
	i         int      // field index in struct
	is        []int
	tag       string
	omitEmpty bool
	encName   string   // encode name
	encNameBs []byte
	name      string   // field name
}

type structFieldInfos struct {
	sis []*structFieldInfo
}

func (si *structFieldInfo) field(struc reflect.Value) (rv reflect.Value) {
	if si.i > -1 {
		rv = struc.Field(si.i)
	} else {
		rv = struc.FieldByIndex(si.is)
	}
	return
}

// linear search. faster than binary search in my testing up to 16-field structs.
func (sis *structFieldInfos) getForEncName(name string) (si *structFieldInfo) {
	for _, si = range sis.sis {
		if si.encName == name {
			return
		}
	}
	si = nil
	return
}

func getStructFieldInfos(rt reflect.Type) (sis *structFieldInfos) {
	sis, ok := cachedStructFieldInfos[rt]
	if ok {
		return 
	}
	
	cachedStructFieldInfosMutex.Lock()
	defer cachedStructFieldInfosMutex.Unlock()
	
	sis = new(structFieldInfos)
	
	var siInfo *structFieldInfo
	if f, ok := rt.FieldByName(structInfoFieldName); ok {
		siInfo = parseStructFieldInfo(structInfoFieldName, f.Tag.Get("msgpack"))
	}
	rgetStructFieldInfos(rt, nil, sis, siInfo)
	cachedStructFieldInfos[rt] = sis
	return
}

func rgetStructFieldInfos(rt reflect.Type, indexstack []int, sis *structFieldInfos, siInfo *structFieldInfo) {
	for j := 0; j < rt.NumField(); j++ {
		f := rt.Field(j)
		stag := f.Tag.Get("msgpack")
		if stag == "-" {
			continue
		}

		if r1, _ := utf8.DecodeRuneInString(f.Name); r1 == utf8.RuneError || !unicode.IsUpper(r1) {
			continue
		} 

		if f.Anonymous {
			//if anonymous, inline it if there is no msgpack tag, else treat as regular field
			if stag == "" {
				rgetStructFieldInfos(f.Type, append2Is(indexstack, j), sis, siInfo)
				continue
			}
		}
		si := parseStructFieldInfo(f.Name, stag)
		
		if len(indexstack) == 0 {
			si.i = j
		} else {
			si.i = -1
			si.is = append2Is(indexstack, j)
		}

		if siInfo != nil {
			if siInfo.omitEmpty {
				si.omitEmpty = true
			}
		}
		sis.sis = append(sis.sis, si)
	}
}

func append2Is(indexstack []int, j int) (indexstack2 []int) {
	// istack2 := indexstack //make copy (not sufficient ... since it'd still share array)
	indexstack2 = make([]int, len(indexstack)+1)
	copy(indexstack2, indexstack)
	indexstack2[len(indexstack2)-1] = j
	return
}

func parseStructFieldInfo(fname string, stag string) (si *structFieldInfo) {
	if fname == "" {
		panic("parseStructFieldInfo: No Field Name")
	}
	si = &structFieldInfo {
		name: fname,
		encName: fname,
		tag: stag,
	}	
	
	if stag != "" {
		for i, s := range strings.Split(si.tag, ",") {
			if i == 0 {
				if s != "" {
					si.encName = s
				}
			} else {
				if s == "omitempty" {
					si.omitEmpty = true
				}
			}
		}
	}
	si.encNameBs = []byte(si.encName)
	return
}

func getContainerByteDesc(ct ContainerType) (cutoff int, b0, b1, b2 byte) {
	switch ct {
	case ContainerRawBytes:
		cutoff = 32
		b0, b1, b2 = 0xa0, 0xda, 0xdb
	case ContainerList:
		cutoff = 16
		b0, b1, b2 = 0x90, 0xdc, 0xdd
	case ContainerMap:
		cutoff = 16
		b0, b1, b2 = 0x80, 0xde, 0xdf
	default:
		panic(fmt.Errorf("getContainerByteDesc: Unknown container type: %v", ct))
	}
	return
}

func reflectValue(v interface{}) (rv reflect.Value) {
	rv, ok := v.(reflect.Value)
	if !ok {
		rv = reflect.ValueOf(v)
	}
	return 
}

func panicToErr(err *error) {
	if x := recover(); x != nil { 
		panicToErrT(x, err)
	}
}

func doPanic(tag string, format string, params []interface{}) {
	params2 := make([]interface{}, len(params) + 1)
	params2[0] = tag
	copy(params2[1:], params)
	panic(fmt.Errorf("%s: " + format, params2...))
}

