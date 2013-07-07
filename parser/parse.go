// Copyright (c) 2013 Uwe Hoffmann. All rights reserved.

/*
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package parser

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"github.com/uwedeportivo/romba/types"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

type parser struct {
	ll *lexer
	d  *types.Dat
}

func (p *parser) consumeStringValue() (string, error) {
	i := p.ll.nextItem()
	switch {
	case i.typ == itemQuotedString:
		return i.val[1 : len(i.val)-1], nil
	case i.typ == itemValue:
		return i.val, nil
	case i.typ > itemValue:
		return i.val, nil
	default:
		return "", fmt.Errorf("expected quoted string or value, got %v", i)
	}
}

func stringValue2Int(input string) (int, error) {
	if input == "-" {
		return 0, nil
	}
	return strconv.Atoi(input)
}

func stringValue2Bytes(input string, expectedLength int) ([]byte, error) {
	if input == "-" || input == "" {
		return nil, nil
	}

	input = strings.TrimSpace(input)

	if len(input) < expectedLength {
		input = strings.Repeat("0", expectedLength-len(input)) + input
	}

	return hex.DecodeString(input)
}

func (p *parser) consumeIntegerValue() (int, error) {
	i := p.ll.nextItem()
	if i.typ == itemValue {
		return stringValue2Int(i.val)
	}
	if i.typ == itemQuotedString {
		return stringValue2Int(i.val[1 : len(i.val)-1])
	}
	return 0, fmt.Errorf("expected value, got %v", i)
}

func (p *parser) consumeHexBytes(expectedLength int) ([]byte, error) {
	i := p.ll.nextItem()
	if i.typ == itemValue {
		return stringValue2Bytes(i.val, expectedLength)
	}
	if i.typ == itemQuotedString {
		return stringValue2Bytes(i.val[1:len(i.val)-1], expectedLength)
	}
	return nil, fmt.Errorf("expected value, got %v", i)
}

func (p *parser) datStmt() error {
	i := p.ll.nextItem()
	err := p.match(i, itemOpenBrace)
	if err != nil {
		return err
	}

	for i = p.ll.nextItem(); i.typ != itemCloseBrace && i.typ != itemEOF && i.typ != itemError; i = p.ll.nextItem() {
		switch {
		case i.typ == itemName:
			p.d.Name, err = p.consumeStringValue()
			if err != nil {
				return err
			}
		case i.typ == itemDescription:
			p.d.Description, err = p.consumeStringValue()
			if err != nil {
				return err
			}
		}
	}

	if i.typ == itemEOF {
		return fmt.Errorf("unexpected end of input")
	}
	if i.typ == itemError {
		return lexError(i)
	}
	return nil
}

func lexError(i item) error {
	return fmt.Errorf("lexer error: %v", i)
}

func (p *parser) gameStmt() (*types.Game, error) {
	i := p.ll.nextItem()
	err := p.match(i, itemOpenBrace)
	if err != nil {
		return nil, err
	}

	g := &types.Game{}

	for i = p.ll.nextItem(); i.typ != itemCloseBrace && i.typ != itemEOF && i.typ != itemError; i = p.ll.nextItem() {
		switch {
		case i.typ == itemName:
			g.Name, err = p.consumeStringValue()
			if err != nil {
				return nil, err
			}
		case i.typ == itemDescription:
			g.Description, err = p.consumeStringValue()
			if err != nil {
				return nil, err
			}
		case i.typ == itemRom:
			r, err := p.romStmt()
			if err != nil {
				return nil, err
			}

			if r != nil {
				g.Roms = append(g.Roms, r)
			}
		}
	}

	if i.typ == itemEOF {
		return nil, fmt.Errorf("unexpected end of input")
	}
	if i.typ == itemError {
		return nil, lexError(i)
	}
	return g, nil
}

func (p *parser) romStmt() (*types.Rom, error) {
	i := p.ll.nextItem()
	err := p.match(i, itemOpenBrace)
	if err != nil {
		return nil, err
	}

	r := &types.Rom{}

	for i = p.ll.nextItem(); i.typ != itemCloseBrace && i.typ != itemEOF && i.typ != itemError; i = p.ll.nextItem() {
		switch {
		case i.typ == itemName:
			r.Name, err = p.consumeStringValue()
			if err != nil {
				return nil, err
			}
		case i.typ == itemSize:
			r.Size, err = p.consumeIntegerValue()
			if err != nil {
				return nil, err
			}
		case i.typ == itemMd5:
			r.Md5, err = p.consumeHexBytes(32)
			if err != nil {
				return nil, nil
			}
		case i.typ == itemCrc:
			r.Crc, err = p.consumeHexBytes(8)
			if err != nil {
				return nil, nil
			}
		case i.typ == itemSha1:
			r.Sha1, err = p.consumeHexBytes(40)
			if err != nil {
				return nil, nil
			}
		}
	}

	if i.typ == itemEOF {
		return nil, fmt.Errorf("unexpected end of input")
	}
	if i.typ == itemError {
		return nil, lexError(i)
	}
	return r, nil
}

func (p *parser) parse() error {
	var i item

	for i = p.ll.nextItem(); i.typ != itemEOF && i.typ != itemError; i = p.ll.nextItem() {
		switch {
		case i.typ == itemClrMamePro:
			err := p.datStmt()
			if err != nil {
				return err
			}
		case i.typ == itemGame:
			g, err := p.gameStmt()
			if err != nil {
				return err
			}
			if g != nil {
				p.d.Games = append(p.d.Games, g)
			}
		}
	}
	if i.typ == itemError {
		return lexError(i)
	}
	return nil
}

func (p *parser) match(i item, typ itemType) error {
	if i.typ == typ {
		return nil
	}
	return fmt.Errorf("expected token of type %v, got %v instead", typ, i)
}

func ParseDat(r io.Reader, path string) (*types.Dat, []byte, error) {
	hr := hashingReader{
		ir: r,
		h:  sha1.New(),
	}

	p := &parser{
		ll: lex("dat", hr),
		d:  &types.Dat{},
	}

	err := p.parse()
	if err != nil {
		return nil, nil, fmt.Errorf("error in file %s on line %d: %v", path, p.ll.lineNumber(), err)
	}
	p.d.Normalize()
	return p.d, hr.h.Sum(nil), nil
}

type hashingReader struct {
	ir io.Reader
	h  hash.Hash
}

func (r hashingReader) Read(buf []byte) (int, error) {
	n, err := r.ir.Read(buf)
	if err == nil {
		r.h.Write(buf[:n])
	}
	return n, err
}

type lineCountingReader struct {
	ir   io.Reader
	line int
}

func (r lineCountingReader) Read(buf []byte) (int, error) {
	n, err := r.ir.Read(buf)
	if err == nil {
		for _, b := range buf[:n] {
			if b == '\n' {
				r.line++
			}
		}
	}
	return n, err
}

func isXML(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	lr := io.LimitedReader{
		R: file,
		N: 256,
	}

	snippet, err := ioutil.ReadAll(&lr)
	if err != nil {
		return false, err
	}

	return strings.HasPrefix(string(snippet), "<?xml"), nil
}

func Parse(path string) (*types.Dat, []byte, error) {
	isXML, err := isXML(path)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	if isXML {
		return ParseXml(file, path)
	}
	return ParseDat(file, path)
}

func fixHashes(rom *types.Rom) {
	if rom.Crc != nil {
		v, err := hex.DecodeString(string(rom.Crc))
		if err != nil {
			rom.Crc = nil
		}
		rom.Crc = v
	}
	if rom.Md5 != nil {
		v, err := hex.DecodeString(string(rom.Md5))
		if err != nil {
			rom.Md5 = nil
		}
		rom.Md5 = v
	}
	if rom.Sha1 != nil {
		v, err := hex.DecodeString(string(rom.Sha1))
		if err != nil {
			rom.Sha1 = nil
		}
		rom.Sha1 = v
	}
}

func ParseXml(r io.Reader, path string) (*types.Dat, []byte, error) {
	br := bufio.NewReader(r)

	hr := hashingReader{
		ir: br,
		h:  sha1.New(),
	}

	lr := lineCountingReader{
		ir: hr,
	}

	d := new(types.Dat)
	decoder := xml.NewDecoder(lr)

	err := decoder.Decode(d)
	if err != nil {
		return nil, nil, fmt.Errorf("xml parsing error %d: %v", lr.line, err)
	}

	for _, g := range d.Games {
		for _, rom := range g.Roms {
			fixHashes(rom)
		}
		for _, rom := range g.Disks {
			fixHashes(rom)
		}
		for _, rom := range g.Parts {
			fixHashes(rom)
		}
		for _, rom := range g.Regions {
			fixHashes(rom)
		}
	}

	for _, g := range d.Software {
		for _, rom := range g.Roms {
			fixHashes(rom)
		}
		for _, rom := range g.Disks {
			fixHashes(rom)
		}
		for _, rom := range g.Parts {
			fixHashes(rom)
		}
		for _, rom := range g.Regions {
			fixHashes(rom)
		}
	}

	d.Normalize()
	return d, hr.h.Sum(nil), nil
}
