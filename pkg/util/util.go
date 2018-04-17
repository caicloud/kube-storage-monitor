package util

import (
	esutil "github.com/kubernetes-incubator/external-storage/lib/util"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/volume/util"
)

// RoundDownCapacityPretty rounds down the capacity to an easy to read value.
func RoundDownCapacityPretty(capacityBytes int64) int64 {
	easyToReadUnitsBytes := []int64{esutil.GiB, esutil.MiB}

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

// GetDirUsageByte returns usage in bytes about a mounted filesystem.
// fullPath is the pathname of any file within the mounted filesystem. Usage
// returned here is block being used * fragment size (aka block size).
func GetDirUsageByte(fullPath string) (*resource.Quantity, error) {
	usage, err := util.Du(fullPath)
	return usage, err
}
