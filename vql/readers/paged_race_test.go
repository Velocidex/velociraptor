package readers

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/vfilter"

	_ "www.velocidex.com/golang/velociraptor/accessors/file"
)

// Regression test for the foreach(workers=N) evidence-corruption race.
//
// The ReaderPool hands every caller a reader out of a size-limited
// cache. When that cache is over its size limit an insert evicts an
// entry and the eviction callback closes that reader from a background
// goroutine. Nothing coordinated that close with a read that was already
// in flight: AccessorReader.ReadAt used to grab self.paged_reader under
// the mutex and then RELEASE the mutex before reading through it, so the
// eviction could close the underlying os.File out from under an active
// read. The read then failed with os.ErrClosed.
//
// That is invisible to callers who only check "did I get an object":
// authenticode() turns a failed read of the certificate table into a
// result with SubjectName/IssuerName null and Trusted "untrusted" and no
// log line at all.
//
// Serially there is only ever one reader in flight and it is the one just
// inserted, so this never fires - which is why the corruption only ever
// showed up under foreach(workers=N).
//
// This test reproduces the defect WITHOUT any Windows API: it is pure
// pool + accessor behaviour, so it runs (and fails on the unfixed code)
// on every platform.

const (
	raceNumFiles = 200

	// Each file is big enough that a full read is many page faults, so
	// there is a real window during which the handle can be closed.
	raceFileSize = 512 * 1024

	// Deliberately NOT a multiple of the 8K page size, so PagedReader
	// walks its page loop instead of delegating the whole read to the
	// underlying reader in one syscall.
	raceChunkSize = 4000

	raceGoroutines = 8
	raceIterations = 6

	// Much smaller than the working set, so nearly every open evicts
	// somebody. Production uses 50 against thousands of binaries.
	racePoolSize = 5
)

type RaceTestSuite struct {
	suite.Suite
	scope     vfilter.Scope
	tmp_dir   string
	filenames []*accessors.OSPath
	pool      *ReaderPool
}

// Byte i of file n is deterministic, so a short/garbled read is
// detectable and not just an error.
func raceByteAt(file_idx, offset int) byte {
	return byte((file_idx*7 + offset*3) & 0xff)
}

func (self *RaceTestSuite) SetupTest() {
	self.scope = vql_subsystem.MakeScope()
	self.scope.AppendVars(ordereddict.NewDict().
		Set(vql_subsystem.ACL_MANAGER_VAR, acl_managers.NullACLManager{}).
		Set(constants.SCOPE_ROOT, self.scope))

	// Establish the pool at a small size limit before anything calls
	// NewAccessorReader (which would create it at its own default).
	self.pool = GetReaderPool(self.scope, racePoolSize)

	var err error
	self.tmp_dir, err = tempfile.TempDir("tmp")
	assert.NoError(self.T(), err)

	accessor, err := accessors.GetAccessor("file", self.scope)
	assert.NoError(self.T(), err)

	self.filenames = make([]*accessors.OSPath, 0, raceNumFiles)
	for i := 0; i < raceNumFiles; i++ {
		dir, err := accessor.ParsePath(self.tmp_dir)
		assert.NoError(self.T(), err)

		file := dir.Append(fmt.Sprintf("race%03d.bin", i))
		self.filenames = append(self.filenames, file)

		buff := make([]byte, raceFileSize)
		for j := range buff {
			buff[j] = raceByteAt(i, j)
		}

		err = os.WriteFile(file.String(), buff, 0600)
		assert.NoError(self.T(), err)
	}
}

func (self *RaceTestSuite) TearDownTest() {
	self.scope.Close()
	os.RemoveAll(self.tmp_dir)
}

// readWholeFile reads file_idx end to end through a pooled reader and
// reports the first failure it sees.
func (self *RaceTestSuite) readWholeFile(file_idx int) error {
	reader, err := NewAccessorReader(
		self.scope, "file", self.filenames[file_idx], 100)
	if err != nil {
		return fmt.Errorf("file %d: NewAccessorReader: %w", file_idx, err)
	}
	// Mirrors what authenticode()/parse_pe() do with a pooled reader.
	defer reader.Close()

	buff := make([]byte, raceChunkSize)
	for offset := 0; offset+raceChunkSize <= raceFileSize; offset += raceChunkSize {
		n, err := reader.ReadAt(buff, int64(offset))
		if err != nil {
			return fmt.Errorf(
				"file %d offset %d: ReadAt: %w", file_idx, offset, err)
		}
		if n != raceChunkSize {
			return fmt.Errorf("file %d offset %d: short read %d",
				file_idx, offset, n)
		}
		for j := 0; j < raceChunkSize; j++ {
			if expected := raceByteAt(file_idx, offset+j); buff[j] != expected {
				return fmt.Errorf(
					"file %d offset %d: byte %d is %#x, expected %#x",
					file_idx, offset, j, buff[j], expected)
			}
		}
	}
	return nil
}

// Serial control. This is what distinguishes a concurrency defect from
// an environment problem: if this fails the test itself is wrong.
func (self *RaceTestSuite) TestSerialBaseline() {
	for i := 0; i < raceNumFiles; i++ {
		assert.NoError(self.T(), self.readWholeFile(i))
	}
}

// The reproducer. Fails on the unfixed pool with os.ErrClosed
// ("file already closed") from a read whose handle was closed by an
// unrelated eviction running in the background.
func (self *RaceTestSuite) TestConcurrentReadsSurviveEviction() {
	var wg sync.WaitGroup
	var failures int64

	errs := make(chan error, raceGoroutines*raceIterations*raceNumFiles)

	for g := 0; g < raceGoroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()

			for iter := 0; iter < raceIterations; iter++ {
				for i := 0; i < raceNumFiles; i++ {
					// Stagger the goroutines so they work on
					// different files at the same time, which is what
					// generates the eviction pressure.
					file_idx := (i + g*raceNumFiles/raceGoroutines) % raceNumFiles

					if err := self.readWholeFile(file_idx); err != nil {
						if atomic.AddInt64(&failures, 1) <= 20 {
							errs <- err
						}
					}
				}
			}
		}(g)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		self.T().Logf("concurrent read failure: %v", err)
	}

	total := int64(raceGoroutines * raceIterations * raceNumFiles)
	assert.Equal(self.T(), int64(0), atomic.LoadInt64(&failures),
		fmt.Sprintf("%d/%d concurrent pooled reads were corrupted or "+
			"failed; a pooled reader was closed by another goroutine's "+
			"cache eviction while this read was in flight",
			atomic.LoadInt64(&failures), total))
}

func TestReaderPoolRace(t *testing.T) {
	suite.Run(t, &RaceTestSuite{})
}
