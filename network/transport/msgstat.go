package transport

import (
	"strconv"
	"strings"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
)

// StatSendMsg records metrics for a sent message.
func StatSendMsg(pkg *TransSendPkg, bufSize, tailSize int) {
	msgID := pkg.PkgHdr.GetMsgID()
	if pkg.PkgHdr.GetRetCode() != 0 {
		metrics.IncrCounterWithDimGroup(metrics.NameMsglayerHandleRetCodeTotal, metrics.GroupStrix, 1,
			metrics.Dimension{
				metrics.DimMsgID:   msgID,
				metrics.DimRetCode: strconv.Itoa(int(pkg.PkgHdr.GetRetCode())),
			},
		)
	}

	dim := GetmetricsMsgDimCache(msgID)
	metrics.IncrCounterWithDimGroup(metrics.NameTransportSendMsgTotal, metrics.GroupStrix,
		1, dim)

	// 统计消息大小
	msgSizeKB := float64(bufSize-tailSize) / metrics.KB
	if strings.HasPrefix(msgID, "CS") || strings.HasPrefix(msgID, "SC") {
		metrics.UpdateAvgGaugeWithDimGroup(metrics.NameTransportCSMsgBodySizeAvgKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
		metrics.UpdateMaxGaugeWithDimGroup(metrics.NameTransportCSMsgSizeMaxKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
		if tailSize > 0 {
			metrics.UpdateMaxGaugeWithDimGroup(metrics.NameTransportCSMsgTailSizeMaxKB,
				metrics.GroupStrix, metrics.Value(tailSize)/metrics.KB, dim)
		}
	} else {
		metrics.UpdateAvgGaugeWithDimGroup(metrics.NameTransportSSMsgSizeAvgKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
		metrics.UpdateMaxGaugeWithDimGroup(metrics.NameTransportSSMsgSizeMaxKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
		metrics.IncrCounterWithDimGroup(metrics.NameTransportSSMsgTotalKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
	}
}

// StatRecvMsg records metrics for a received message. It only records the size of CS messages,
// as SS messages are internal to the cluster and are already recorded by StatSendMsg to avoid duplication.
func StatRecvMsg(pkg *TransRecvPkg, msgSize int) {

	msgID := pkg.PkgHdr.GetMsgID()
	dim := GetmetricsMsgDimCache(msgID)
	metrics.IncrCounterWithDimGroup(
		metrics.NameTransportRecvMsgTotal,
		metrics.GroupStrix,
		1,
		dim,
	)
	if !strings.HasPrefix(msgID, "CS") && !strings.HasPrefix(msgID, "SC") {
		return
	}
	msgSizeKB := float64(msgSize) / metrics.KB
	metrics.UpdateAvgGaugeWithDimGroup(metrics.NameTransportCSMsgBodySizeAvgKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
	metrics.UpdateMaxGaugeWithDimGroup(metrics.NameTransportCSMsgSizeMaxKB, metrics.GroupStrix, metrics.Value(msgSizeKB), dim)
}

var _metricsMsgDimCacheMap = make(map[string]metrics.Dimension)

// RegisterMsgDim registers the metric dimension for a message.
func RegisterMsgDim(msgID string) {
	val := metrics.Dimension{
		metrics.DimMsgID: msgID,
	}
	_metricsMsgDimCacheMap[msgID] = val
}

// GetmetricsMsgDimCache get metrics msg cache.
func GetmetricsMsgDimCache(msgID string) metrics.Dimension {
	val, ok := _metricsMsgDimCacheMap[msgID]
	if ok {
		return val
	}

	// Note: This log might spam, but theoretically, it should not be reached.
	// If this log appears, it needs to be fixed.
	log.Warn().Str("msg", msgID).Msg("dim no cache for")
	return metrics.Dimension{
		metrics.DimMsgID: msgID,
	}
}
