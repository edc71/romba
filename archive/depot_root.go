package archive

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/golang/glog"
	"github.com/willf/bloom"
)

type depotRoot struct {
	sync.Mutex

	path       string
	bloomReady bool
	bf         *bloom.BloomFilter
	touched    bool
	size       int64
	maxSize    int64

	numBfAdded int64
}

func loadBloomFilter(root string, bf *bloom.BloomFilter) error {
	bfp := filepath.Join(root, bloomFilterFilename)
	exists, err := PathExists(bfp)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	file, err := os.Open(bfp)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = bf.ReadFrom(file)
	return err
}

func writeBloomFilter(path string, bf *bloom.BloomFilter) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = bf.WriteTo(file)
	return err
}

func writeBloomFilterWithBackup(root string, bf *bloom.BloomFilter) error {
	bfFilePath := filepath.Join(root, bloomFilterFilename)

	exists, err := PathExists(bfFilePath)
	if err != nil {
		return err
	}

	if exists {
		backupBfFilePath := filepath.Join(root, backupBloomFilterFilename)

		err := os.Rename(bfFilePath, backupBfFilePath)
		if err != nil {
			return err
		}
	}

	file, err := os.Create(bfFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = bf.WriteTo(file)
	return err
}

func (depot *Depot) writeSizes() {
	for _, dr := range depot.roots {
		dr.Lock()
		if dr.touched {
			err := writeSizeFile(dr.path, dr.size)
			if err != nil {
				glog.Errorf("failed to write size file into %s: %v\n", dr.path, err)
			} else {
				dr.touched = false
			}

			if dr.bloomReady {
				err = writeBloomFilterWithBackup(dr.path, dr.bf)
				if err != nil {
					dr.touched = true
					glog.Errorf("failed to write bloomfilter into %s: %v\n", dr.path, err)
				}
			}
		}
		dr.Unlock()
	}
}

func (depot *Depot) adjustSize(index int, delta int64, sha1Hex string) {
	dr := depot.roots[index]
	dr.Lock()
	defer dr.Unlock()

	dr.size += delta

	if dr.size < 0 {
		dr.size = 0
	}

	if sha1Hex != "" && dr.bloomReady {
		dr.bf.Add([]byte(sha1Hex))
	}

	dr.touched = true
}
