// Package metrics defines the types and constants used for metric collection and reporting.
package metrics

// Policy defines the aggregation policy for metric values.
// It determines how multiple values for the same metric should be combined over a time window.
type Policy int

const (
	Policy_None      Policy = iota // Policy_None indicates no specific aggregation policy. The reporting system may use a default.
	Policy_Set                     // Policy_Set represents an instantaneous value; the last reported value wins.
	Policy_Sum                     // Policy_Sum represents a cumulative value, summing all reported values.
	Policy_Avg                     // Policy_Avg represents the average of all reported values.
	Policy_Max                     // Policy_Max represents the maximum value among all reported values.
	Policy_Min                     // Policy_Min represents the minimum value among all reported values.
	Policy_Mid                     // Policy_Mid represents the median value among all reported values.
	Policy_Stopwatch               // Policy_Stopwatch is for timing metrics, measuring event durations.
	Policy_Histogram               // Policy_Histogram is for histogram statistics, capturing value distribution.
)

// Value represents a metric value as a float64.
type Value float64

// Dimension represents metric dimensions as key-value pairs.
// Dimensions provide contextual information for metrics, such as server name, region, or version.
type Dimension map[string]string

const (
	// KB represents a kilobyte (1024 bytes).
	KB = 1024.0
	// MB represents a megabyte (1024 * 1024 bytes).
	MB = 1024.0 * 1024.0
)

// compdim: Metric Comparison Rules
// The `compdim` comment tag defines rules for comparing metric data between different dimensions for automated analysis and alerting.
//
// Format:
//   compdim:dimension1:rule1[&|]rule2,dimension2:rule1[&|]rule2
//
// Pre-defined Rules:
//
//   - Wave: Fluctuation Rule
//     - Config: `Wave15%` (percentage is dynamic)
//     - Description: Compares the fluctuation of data between two dimensions.
//     - Calculation: `actual_fluctuation = difference / smaller_value`
//     - Trigger: Activates if `actual_fluctuation >= configured_fluctuation`.
//
//   - Base: Threshold Rule
//     - Config: `Base15` (threshold is dynamic)
//     - Description: Activates if the data from either of the two dimensions is greater than or equal to the threshold.
//
//   - ActorWave: Actor-Normalized Fluctuation Rule
//     - Config: `ActorWave15%` (percentage is dynamic)
//     - Description: Normalizes data by the number of actors in the time window before comparing fluctuation.
//     - Calculation:
//       - `dim1_actual_value = dim1_data / dim1_actor_count`
//       - `dim2_actual_value = dim2_data / dim2_actor_count`
//       - `actual_fluctuation = difference_between_actual_values / smaller_actual_value`
//     - Trigger: Activates if `actual_fluctuation >= configured_fluctuation`.

// Group related constants, prefixed with Group.
const (
	// GroupStrix is the group name for strix-related metrics.
	GroupStrix = "strix"
)

// Metric related constants
const (
	// NamePoolCreateTotal: Total number of objects created by a pool because the pool was empty.
	// group:strix dimension:poolname owner:jugglewang compdim:poolname:ActorWave20%
	NamePoolCreateTotal = "pool_create_total"

	// NameMsglayerHandleRetCodeTotal: Total number of non-zero return codes from synchronous message handling.
	// group:strix dimension:msgid,retcode dashboard:Total non-zero return codes from message handling (excluding async). alarm:Alert on any error code from 1-1000, or >100% upward fluctuation for codes >1000. owner:jugglewang compdim:msgid:ActorWave20%,retcode:ActorWave20%
	NameMsglayerHandleRetCodeTotal = "msglayer_handle_retcode_total"

	// NameTransportSendMsgTotal: Total number of messages sent by the transport layer.
	// group:strix dimension:msgid dashboard:Total number of sent messages. alarm:Upward fluctuation >100%, downward >50%. owner:jonnymiao compdim:msgid:ActorWave20%
	NameTransportSendMsgTotal = "transport_send_total"

	// NameTransportCSMsgBodySizeAvgKB: Average body size of client-to-server messages in KB.
	// group:strix dimension:msgid dashboard:Average body size of CS messages. owner:jonnymiao compdim:msgid:Wave20%&Base0.1&SkipNone
	NameTransportCSMsgBodySizeAvgKB = "transport_csmsg_body_avg_bytes"

	// NameTransportCSMsgSizeMaxKB: Maximum size of a client-to-server message in KB.
	// group:strix dimension:msgid dashboard:Maximum size of a CS message. owner:jonnymiao compdim:msgid:ActorWave20%&Base0.1&SKipNone
	NameTransportCSMsgSizeMaxKB = "transport_csmsg_size_max_KB"

	// NameTransportCSMsgTailSizeMaxKB: Maximum tail size of a client-to-server message in KB.
	// group:strix dimension:msgid dashboard:Maximum tail size of a CS message. owner:jonnymiao compdim:msgid:Wave20%&Base0.1|SkipNone
	NameTransportCSMsgTailSizeMaxKB = "transport_csmsg_tail_max_KB"

	// NameTransportSSMsgSizeAvgKB: Average size of a single server-to-server message in KB.
	// group:strix dimension:msgid dashboard:Average size of a single SS message. owner:jonnymiao compdim:msgid:Wave20%&Base0.1&SKipNone
	NameTransportSSMsgSizeAvgKB = "transport_ssmsg_size_avg_KB"

	// NameTransportSSMsgSizeMaxKB: Maximum size of a single server-to-server message in KB.
	// group:strix dimension:msgid dashboard:Maximum size of a single SS message. owner:jonnymiao compdim:msgid:Wave20%&Base0.1&SkipNone
	NameTransportSSMsgSizeMaxKB = "transport_ssmsg_size_max_KB"

	// NameTransportSSMsgTotalKB: Total size of all sent server-to-server messages in KB.
	// group:strix dimension:msgid dashboard:Total size of all sent SS messages. owner:jonnymiao compdim:msgid:ActorWave20%&Base0.1&SKipNone
	NameTransportSSMsgTotalKB = "transport_ssmsg_size_total_KB"

	// NameTransportRecvMsgTotal: Total number of messages received by the transport layer.
	// group:strix dimension:msgid dashboard:Total number of received messages. alarm:Upward fluctuation >100%, downward >50%. owner:jonnymiao compdim:msgid:ActorWave20%
	NameTransportRecvMsgTotal = "transport_recv_total"

	// NameSidecarReadMemUsageMaxPercent: Maximum memory usage percentage of a single process's sidecar read queue.
	// group:strix dimension: dashboard:Max memory usage percentage of a single process's read queue. alarm:Exceeds 15%. owner:jonnymiao compdim:funcid:ActorWave20%&Base0.1&SkipNone
	NameSidecarReadMemUsageMaxPercent = "sidecar_read_mem_usage_percent"

	// NameSidecarWriteMemUsageMaxPercent: Maximum memory usage percentage of a single process's sidecar write queue.
	// group:strix dimension: dashboard:Max memory usage percentage of a single process's write queue. alarm:Exceeds 15%. owner:jonnymiao compdim:funcid:ActorWave20%&Base0.1&SkipNone
	NameSidecarWriteMemUsageMaxPercent = "sidecar_write_mem_usage_percent"

	// NameSidecarReadDelayMaxMS: Maximum delay of a single process's sidecar read queue in milliseconds.
	// group:strix dimension: dashboard:Max delay of a single process's read queue. alarm:Exceeds 50ms. owner:jonnymiao compdim:funcid:Wave20%&Base5
	NameSidecarReadDelayMaxMS = "sidecar_read_delay_max_ms"

	// NameSidecarReadDelayAvgMS: Average delay of a single process's sidecar read queue in milliseconds.
	// group:strix dimension: dashboard:Average delay of a single process's read queue. alarm:Exceeds 30ms. owner:jonnymiao compdim:funcid:Wave20%&Base5
	NameSidecarReadDelayAvgMS = "sidecar_read_delay_avg_ms"
)

// Dimension related definitions, must be prefixed with Dim. The comment should include the group.
const (
	// DimMsgID is the dimension for message ID.
	// group:strix
	DimMsgID = "msgid"
	// DimRetCode is the dimension for return code.
	// group:strix
	DimRetCode = "retcode"
	// DimPoolName is the dimension for pool name.
	// group:strix
	DimPoolName = "poolname"
	// DimPipeIdx is the dimension for pipe index.
	// group:strix
	DimPipeIdx = "idx"
)
