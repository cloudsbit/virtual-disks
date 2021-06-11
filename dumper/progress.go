package dumper

import "sync"

type DiskProgress struct {
	sync.RWMutex
	Capacity 	uint64
	Processed  	uint64
	Estimate 	uint64
	Finished	uint64
}

func (dp *DiskProgress) SetCapacitySize(cap uint64) {
	dp.Lock()
	dp.Capacity = cap
	dp.Unlock()
}

func (dp *DiskProgress) GetCapacitySize() uint64 {
	dp.RLock()
	defer dp.RUnlock()
	return dp.Capacity
}

func (dp *DiskProgress) SetProcessedSize(pro uint64) {
	dp.Lock()
	dp.Processed = pro
	dp.Unlock()
}

func (dp *DiskProgress) GetProcessedSize() uint64 {
	dp.RLock()
	defer dp.RUnlock()
	return dp.Processed
}

func (dp *DiskProgress) SetEstimateSize(estSize uint64) {
	dp.Lock()
	dp.Estimate = estSize
	dp.Unlock()
}

func (dp *DiskProgress) GetEstimateSize() uint64 {
	dp.RLock()
	defer dp.RUnlock()
	return dp.Estimate
}

func (dp *DiskProgress) SetFinishedSize(size uint64) {
	dp.Lock()
	defer dp.Unlock()
	dp.Finished = size
}

func (dp *DiskProgress) UpdateFinishedSize(size uint64) {
	dp.Lock()
	defer dp.Unlock()
	dp.Finished += size
}


func (dp *DiskProgress) GetFinishedSize() uint64 {
	dp.RLock()
	defer dp.RUnlock()
	return dp.Finished
}


//
// 暂时放这里的
//

func GetEstimateSize(dc *DiskChangeInfo) uint64 {
	var estimate uint64
	for _, area := range dc.ChangedArea {
		estimate += uint64(area.Length)
	}
	return estimate
}






