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

package db

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"

	"github.com/uwedeportivo/romba/parser"
	"github.com/uwedeportivo/romba/types"
	"github.com/uwedeportivo/romba/worker"
)

const (
	generationFilename = "romba-generation"
	MaxBatchSize       = 10485760
)

type RomBatch interface {
	IndexRom(rom *types.Rom) error
	IndexDat(dat *types.Dat, sha1 []byte) error
	Size() int64
	Flush() error
	Close() error
}

type RomDB interface {
	StartBatch() RomBatch
	IndexRom(rom *types.Rom) error
	IndexDat(dat *types.Dat, sha1 []byte) error
	OrphanDats() error
	Flush()
	Close() error
	GetDat(sha1 []byte) (*types.Dat, error)
	DatsForRom(rom *types.Rom) ([]*types.Dat, error)
}

var DBFactory func(path string) (RomDB, error)

func New(path string) (RomDB, error) {
	return DBFactory(path)
}

func WriteGenerationFile(root string, size int64) error {
	file, err := os.Create(filepath.Join(root, generationFilename))
	if err != nil {
		return err
	}
	defer file.Close()

	bw := bufio.NewWriter(file)
	defer bw.Flush()

	bw.WriteString(strconv.FormatInt(size, 10))
	return nil
}

func ReadGenerationFile(root string) (int64, error) {
	file, err := os.Open(filepath.Join(root, generationFilename))
	if err != nil {
		if os.IsNotExist(err) {
			err = WriteGenerationFile(root, 0)
			if err != nil {
				return 0, err
			}
			return 0, nil
		}
		return 0, err
	}
	defer file.Close()

	bs, err := ioutil.ReadAll(file)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(string(bs), 10, 64)
}

type refreshWorker struct {
	romBatch RomBatch
}

func (pw *refreshWorker) Process(path string, size int64) error {
	if pw.romBatch.Size() >= MaxBatchSize {
		glog.Infof("flushing batch of size %d", pw.romBatch.Size())
		err := pw.romBatch.Flush()
		if err != nil {
			return fmt.Errorf("failed to flush: %v", err)
		}
	}
	dat, sha1Bytes, err := parser.Parse(path)
	if err != nil {
		return err
	}
	return pw.romBatch.IndexDat(dat, sha1Bytes)
}

func (pw *refreshWorker) Close() error {
	err := pw.romBatch.Close()
	pw.romBatch = nil
	return err
}

type refreshMaster struct {
	romdb      RomDB
	numWorkers int
	pt         worker.ProgressTracker
}

func (pm *refreshMaster) Accept(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".dat" || ext == ".xml"
}

func (pm *refreshMaster) NewWorker(workerIndex int) worker.Worker {
	return &refreshWorker{
		romBatch: pm.romdb.StartBatch(),
	}
}

func (pm *refreshMaster) NumWorkers() int {
	return pm.numWorkers
}

func (pm *refreshMaster) ProgressTracker() worker.ProgressTracker {
	return pm.pt
}

func (pm *refreshMaster) FinishUp() error {
	pm.romdb.Flush()
	return nil
}

func Refresh(romdb RomDB, datsPath string, numWorkers int, pt worker.ProgressTracker) (string, error) {
	err := romdb.OrphanDats()
	if err != nil {
		return "", err
	}

	pm := &refreshMaster{
		romdb:      romdb,
		numWorkers: numWorkers,
		pt:         pt,
	}

	return worker.Work("refresh dats", []string{datsPath}, pm)
}
