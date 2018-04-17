package util

import esUtil "github.com/kubernetes-incubator/external-storage/lib/util"

// RoundDownCapacityPretty rounds down the capacity to an easy to read value.
func RoundDownCapacityPretty(capacityBytes int64) int64 {
	easyToReadUnitsBytes := []int64{esUtil.GiB, esUtil.MiB}

	// Round down to the nearest easy to read unit
	// such that there are at least 10 units at that size.
	for _, easyToReadUnitBytes := range easyToReadUnitsBytes {
		// Round down the capacity to the nearest unit.
		size := capacityBytes / easyToReadUnitBytes
		if size >= 10 {
			return size * easyToReadUnitBytes
		}
	}
	return capacityBytes
}
