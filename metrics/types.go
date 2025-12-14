package metrics

// Policy defines the aggregation policy for metric values.
// It determines how multiple values for the same metric should be combined over time.
type Policy int

const (
	Policy_None      Policy = iota // No specific policy specified
	Policy_Set                     // Instantaneous value - last value wins
	Policy_Sum                     // Sum of all values
	Policy_Avg                     // Average of all values
	Policy_Max                     // Maximum value
	Policy_Min                     // Minimum value
	Policy_Mid                     // Median value
	Policy_Stopwatch               // Timer - measures duration
	Policy_Histogram               // Histogram statistics
)

// Value represents a metric value as a float64.
type Value float64

// Dimension represents metric dimensions as key-value pairs.
// Dimensions are used to add contextual information to metrics,
// such as server name, region, version, etc.
type Dimension map[string]string
