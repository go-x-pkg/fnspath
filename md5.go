package fnspath

import (
	"crypto/md5"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-x-pkg/bufpool"
)

type MD5 struct {
	Latency time.Duration // calculation latency
	Sum     [md5.Size]byte
	Sz      uint64
	B       *bufpool.Buf
}

func (m5 *MD5) Release() {
	m5.B.Release()
	m5.B = nil
}

func (m5 *MD5) Do(path string) error {
	start := time.Now()

	defer func() { m5.Latency = time.Since(start) }()

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	m5.Sz = uint64(fi.Size())

	data, e := ioutil.ReadFile(path)
	if e != nil {
		return nil
	}

	m5.Sum = md5.Sum(data)

	b := m5.B
	if b == nil {
		b = bufpool.NewBuf()
	}

	b.Reset()
	b.Write(data)

	m5.B = b

	return nil
}
