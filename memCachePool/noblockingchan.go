package memCachePool

import (
	"container/list"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/laohanlinux/go-logger/logger"
)

// ThresholdFreeOsMemory (256M) for memCache size to free to os
const (
	ThresholdFreeOsMemory = 268435456
	LifeTimeChan          = 30
)

var noblockOnece sync.Once

var nbc *NoBlockingChan

// memCache Object
type noBuffferObj struct {
	b    []byte
	used int64
}

// NoBlockingChan is a no block channel for memory cache.
// the recycle time is 1 minute ;
// the recycle threshold of total memory is 268435456;
// the recycle threshold of ervry block timeout is 5 minutes
type NoBlockingChan struct {
	send      chan []byte //
	recv      chan []byte //
	freeMem   chan byte   //
	blockSize uint64      //
}

// NewNoBlockingChan for create a no blocking chan bytes with size block
func NewNoBlockingChan(blockSize ...int) *NoBlockingChan {
	noblockOnece.Do(func() {
		logger.Info("do once")
		nbc = &NoBlockingChan{
			send:      make(chan []byte),
			recv:      make(chan []byte),
			freeMem:   make(chan byte),
			blockSize: 1024 * 4,
		}
		go nbc.doWork()
		go nbc.freeOldMemCache()
	})
	return nbc
}

// SendChan ...
func (nbc *NoBlockingChan) SendChan() <-chan []byte { return nbc.send }

// RecycleChan ...
func (nbc *NoBlockingChan) RecycleChan() chan<- []byte { return nbc.recv }

// SetBufferSize used to set no blocking channel into blockSize
func (nbc *NoBlockingChan) SetBufferSize(blockSize uint64) {
	nbc.blockSize = blockSize
}

// Very Block is 4kb
func (nbc *NoBlockingChan) makeBuffer() []byte { return make([]byte, nbc.blockSize) }

func (nbc *NoBlockingChan) bufferSize() uint64 { return 0 }

func (nbc *NoBlockingChan) doWork() {
	defer func() {
		debug.FreeOSMemory()
	}()

	var freeSize uint64
	items := list.New()
	for {
		if items.Len() == 0 {
			items.PushBack(noBuffferObj{
				b:    nbc.makeBuffer(),
				used: time.Now().Unix(),
			})
		}
		e := items.Front()
		select {
		case item := <-nbc.recv:
			items.PushBack(noBuffferObj{
				b:    item,
				used: time.Now().Unix(),
			})
		case nbc.send <- e.Value.(noBuffferObj).b:
			items.Remove(e)
		case <-nbc.freeMem:
			// free too old memcached
			item := items.Front()
			freeTime := time.Now().Unix()
			for item != nil {
				nItem := item.Next()
				if (freeTime - item.Value.(noBuffferObj).used) > LifeTimeChan {
					items.Remove(item)
					item.Value = nil
				} else {
					break
				}
				item = nItem
				freeSize += nbc.blockSize
			}
			// if needed free memory more than ThresholdFreeOsMemory, call the debug.FreeOSMemory
			if freeSize > ThresholdFreeOsMemory {
				fmt.Println("free debug os")
				debug.FreeOSMemory()
				freeSize = 0
			}
		}
	}
}

// free old memcache object, timeout = 1 minute not to be used
func (nbc *NoBlockingChan) freeOldMemCache() {
	//timeout := time.NewTimer(time.Minute * 5)
	timeout := time.NewTicker(time.Second * 60)
	for {
		select {
		case <-timeout.C:
			nbc.freeMem <- 'f'
		}
	}
}