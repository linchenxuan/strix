// Package message defines the core data structures and interfaces for network messages.
// This file specifically implements various routing strategies, providing a flexible
// mechanism for directing messages to their destinations in a distributed system.
package message

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"
)

// Route is the interface for a routing strategy. Implementations of this interface
// define specific algorithms (e.g., P2P, Random, Hash-based) for determining the
// destination of a message.
type Route interface {
	// GetRouteKey returns a string key representing the route. This key can be used
	// for caching routing decisions or for consistent routing.
	GetRouteKey() string
	// GetRouteStrategy returns the type of the routing strategy.
	GetRouteStrategy() RouteStrategy
	// Validate checks if the route's parameters are valid.
	Validate() error
	// String provides a human-readable representation of the route for logging and debugging.
	String() string
}

// RouteStrategy is an enumeration of the available routing strategies.
type RouteStrategy int

const (
	// RouteP2P is for point-to-point messaging to a specific, known entity ID.
	RouteP2P RouteStrategy = iota + 1
	// RouteRand is for sending a message to a random instance of a service, used for load balancing.
	RouteRand
	// RouteHash is for sending a message to a specific instance of a service chosen by hashing a key,
	// used for session stickiness or consistent routing to a shard.
	RouteHash
	// RouteBroadCast is for sending a message to all instances of a service.
	RouteBroadCast
	// RouteMulCast is for sending a message to a specific list of entity IDs.
	RouteMulCast
	// RouteInter is for internal, inter-service communication, typically for RPC calls.
	RouteInter
)

// String returns the string name of the RouteStrategy.
func (r RouteStrategy) String() string {
	switch r {
	case RouteP2P:
		return "P2P"
	case RouteRand:
		return "Random"
	case RouteHash:
		return "Hash"
	case RouteBroadCast:
		return "Broadcast"
	case RouteMulCast:
		return "Multicast"
	case RouteInter:
		return "Internal"
	default:
		return "Unknown"
	}
}

// RouteTypeP2P implements a point-to-point routing strategy.
// It is used to send a message directly to a single, specific entity.
type RouteTypeP2P struct {
	DstEntityID uint64 // The unique identifier of the destination entity.
	routeKey    string // A pre-computed key for this route for performance.
}

// NewRouteTypeP2P creates a new P2P route configuration.
func NewRouteTypeP2P(entityID uint64) *RouteTypeP2P {
	return &RouteTypeP2P{
		DstEntityID: entityID,
		routeKey:    fmt.Sprintf("p2p:%d", entityID),
	}
}

// GetRouteKey returns the unique key for this P2P route.
func (r *RouteTypeP2P) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the P2P strategy type.
func (r *RouteTypeP2P) GetRouteStrategy() RouteStrategy {
	return RouteP2P
}

// Validate checks that the destination entity ID is set.
func (r *RouteTypeP2P) Validate() error {
	if r.DstEntityID == 0 {
		return fmt.Errorf("P2P route has invalid destination entity ID: %d", r.DstEntityID)
	}
	return nil
}

// String provides a human-readable representation of the P2P route.
func (r *RouteTypeP2P) String() string {
	return fmt.Sprintf("P2P[EntityID:%d]", r.DstEntityID)
}

// RouteTypeRand implements a random routing strategy.
// It is used to load-balance requests across all available instances of a service (FuncID).
// It can also perform consistent routing if `UseConstantRoute` is enabled.
type RouteTypeRand struct {
	FuncID           uint32 // The identifier of the target service or function.
	AreaID           uint32 // An optional identifier to scope routing to a specific area/region.
	UseConstantRoute bool   // If true, routing will be consistent based on ConstantKey.
	ConstantKey      uint64 // The key to use for consistent routing when enabled.
	routeKey         string // A pre-computed key for this route.
}

// NewRouteTypeRand creates a new random route configuration.
func NewRouteTypeRand(funcID uint32, areaID uint32, useConstant bool, constantKey uint64) *RouteTypeRand {
	r := &RouteTypeRand{
		FuncID:           funcID,
		AreaID:           areaID,
		UseConstantRoute: useConstant,
		ConstantKey:      constantKey,
	}
	if useConstant {
		r.routeKey = fmt.Sprintf("rand:%d:%d", funcID, constantKey)
	} else {
		r.routeKey = fmt.Sprintf("rand:%d:%d:random", funcID, areaID)
	}
	return r
}

// GetRouteKey returns the unique key for this random route.
func (r *RouteTypeRand) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the Random strategy type.
func (r *RouteTypeRand) GetRouteStrategy() RouteStrategy {
	return RouteRand
}

// Validate checks that the function ID is set.
func (r *RouteTypeRand) Validate() error {
	if r.FuncID == 0 {
		return fmt.Errorf("random route has invalid func ID: %d", r.FuncID)
	}
	return nil
}

// String provides a human-readable representation of the random route.
func (r *RouteTypeRand) String() string {
	if r.UseConstantRoute {
		return fmt.Sprintf("Rand[Func:%d, ConstantKey:%d]", r.FuncID, r.ConstantKey)
	}
	return fmt.Sprintf("Rand[Func:%d, Area:%d]", r.FuncID, r.AreaID)
}

// RouteTypeHash implements a hash-based routing strategy.
// It ensures that messages with the same hash key are consistently sent to the same service instance.
// This is useful for session stickiness or routing to data shards.
type RouteTypeHash struct {
	FuncID   uint32 // The identifier of the target service or function.
	HashKey  string // The string that will be hashed to determine the destination.
	AreaID   uint32 // An optional identifier to scope routing to a specific area/region.
	Replica  int    // The number of replicas for consistent hashing (if applicable).
	routeKey string // A pre-computed key for this route.
}

// NewRouteTypeHash creates a new hash route configuration. It uses the FNV-1a algorithm for hashing.
func NewRouteTypeHash(funcID uint32, hashKey string, areaID uint32, replica int) *RouteTypeHash {
	h := fnv.New64a()
	_, _ = h.Write([]byte(hashKey)) // fnv hash Write never returns an error.
	hash := h.Sum64()

	return &RouteTypeHash{
		FuncID:   funcID,
		HashKey:  hashKey,
		AreaID:   areaID,
		Replica:  replica,
		routeKey: fmt.Sprintf("hash:%d:%d:%d:%d", funcID, areaID, hash, replica),
	}
}

// GetRouteKey returns the unique key for this hash route.
func (r *RouteTypeHash) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the Hash strategy type.
func (r *RouteTypeHash) GetRouteStrategy() RouteStrategy {
	return RouteHash
}

// Validate checks that the function ID and hash key are set.
func (r *RouteTypeHash) Validate() error {
	if r.FuncID == 0 {
		return fmt.Errorf("hash route has invalid func ID: %d", r.FuncID)
	}
	if r.HashKey == "" {
		return fmt.Errorf("hash route has an empty hash key")
	}
	return nil
}

// String provides a human-readable representation of the hash route.
func (r *RouteTypeHash) String() string {
	return fmt.Sprintf("Hash[Func:%d, Key:'%s', Area:%d]", r.FuncID, r.HashKey, r.AreaID)
}

// RouteTypeBroadCast implements a broadcast routing strategy.
// It is used to send a message to all instances of a service, for example, for
// system-wide announcements or cache invalidation.
type RouteTypeBroadCast struct {
	FuncID      uint32 // The identifier of the target service or function.
	AreaID      uint32 // An optional identifier to scope the broadcast to a specific area/region.
	ExcludeSelf bool   // If true, the sending node will be excluded from the broadcast.
	routeKey    string // A pre-computed key for this route.
}

// NewRouteTypeBroadCast creates a new broadcast route configuration.
func NewRouteTypeBroadCast(funcID uint32, areaID uint32, excludeSelf bool) *RouteTypeBroadCast {
	return &RouteTypeBroadCast{
		FuncID:      funcID,
		AreaID:      areaID,
		ExcludeSelf: excludeSelf,
		routeKey:    fmt.Sprintf("broadcast:%d:%d", funcID, areaID),
	}
}

// GetRouteKey returns the unique key for this broadcast route.
func (r *RouteTypeBroadCast) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the Broadcast strategy type.
func (r *RouteTypeBroadCast) GetRouteStrategy() RouteStrategy {
	return RouteBroadCast
}

// Validate checks that the function ID is set.
func (r *RouteTypeBroadCast) Validate() error {
	if r.FuncID == 0 {
		return fmt.Errorf("broadcast route has invalid func ID: %d", r.FuncID)
	}
	return nil
}

// String provides a human-readable representation of the broadcast route.
func (r *RouteTypeBroadCast) String() string {
	return fmt.Sprintf("Broadcast[Func:%d, Area:%d, ExcludeSelf:%v]", r.FuncID, r.AreaID, r.ExcludeSelf)
}

// RouteTypeMulCast implements a multicast routing strategy.
// It is used to send a message to a predefined list of specific entities.
type RouteTypeMulCast struct {
	EntityIDs []uint64 // The list of destination entity identifiers.
	FuncID    uint32   // The service function ID, used for routing context.
	routeKey  string   // A pre-computed key for this route.
}

// NewRouteTypeMulCast creates a new multicast route configuration.
func NewRouteTypeMulCast(funcID uint32, entityIDs []uint64) *RouteTypeMulCast {
	r := &RouteTypeMulCast{
		FuncID:    funcID,
		EntityIDs: entityIDs,
	}

	if len(entityIDs) == 0 {
		r.routeKey = "multicast:empty"
	} else {
		// The key is simplified for brevity. A production implementation might hash the sorted list of IDs.
		r.routeKey = fmt.Sprintf("multicast:%d:%d", funcID, entityIDs[0])
	}
	return r
}

// GetRouteKey returns the unique key for this multicast route.
func (r *RouteTypeMulCast) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the Multicast strategy type.
func (r *RouteTypeMulCast) GetRouteStrategy() RouteStrategy {
	return RouteMulCast
}

// Validate checks that the entity ID list is not empty.
func (r *RouteTypeMulCast) Validate() error {
	if len(r.EntityIDs) == 0 {
		return fmt.Errorf("multicast route has an empty entity list")
	}
	for _, id := range r.EntityIDs {
		if id == 0 {
			return fmt.Errorf("multicast route contains an invalid entity ID: %d", id)
		}
	}
	return nil
}

// String provides a human-readable representation of the multicast route.
func (r *RouteTypeMulCast) String() string {
	return fmt.Sprintf("Multicast[Func:%d, Entities:%d]", r.FuncID, len(r.EntityIDs))
}

// RouteTypeInter implements a routing strategy for internal, inter-service communication.
// It is typically used for RPC calls between different microservices within the system.
type RouteTypeInter struct {
	ServiceName string // The name of the destination service.
	Method      string // The name of the method to be invoked on the service.
	PipeIdx     int32  // An index for load balancing or pipeline selection.
	routeKey    string // A pre-computed key for this route.
}

// NewRouteTypeInter creates a new internal service route configuration.
func NewRouteTypeInter(serviceName string, method string, pipeIdx int32) *RouteTypeInter {
	return &RouteTypeInter{
		ServiceName: serviceName,
		Method:      method,
		PipeIdx:     pipeIdx,
		routeKey:    fmt.Sprintf("inter:%s:%s:%d", serviceName, method, pipeIdx),
	}
}

// GetRouteKey returns the unique key for this internal route.
func (r *RouteTypeInter) GetRouteKey() string {
	return r.routeKey
}

// GetRouteStrategy returns the Internal strategy type.
func (r *RouteTypeInter) GetRouteStrategy() RouteStrategy {
	return RouteInter
}

// Validate checks that the service and method names are set.
func (r *RouteTypeInter) Validate() error {
	if r.ServiceName == "" {
		return fmt.Errorf("internal route has an empty service name")
	}
	if r.Method == "" {
		return fmt.Errorf("internal route has an empty method name")
	}
	return nil
}

// String provides a human-readable representation of the internal route.
func (r *RouteTypeInter) String() string {
	return fmt.Sprintf("Inter[Service:%s, Method:%s, Pipe:%d]", r.ServiceName, r.Method, r.PipeIdx)
}

// RouteBuilder provides a fluent API for constructing Route objects.
// This pattern enhances readability and helps prevent errors in route configuration.
type RouteBuilder struct {
	route Route
}

// NewRouteBuilder creates a new, empty RouteBuilder.
func NewRouteBuilder() *RouteBuilder {
	return &RouteBuilder{}
}

// P2P sets the builder to construct a point-to-point route.
func (b *RouteBuilder) P2P(entityID uint64) *RouteBuilder {
	b.route = NewRouteTypeP2P(entityID)
	return b
}

// Rand sets the builder to construct a random load-balancing route.
func (b *RouteBuilder) Rand(funcID uint32, areaID uint32) *RouteBuilder {
	b.route = NewRouteTypeRand(funcID, areaID, false, 0)
	return b
}

// Hash sets the builder to construct a hash-based consistent route.
func (b *RouteBuilder) Hash(funcID uint32, hashKey string, areaID uint32) *RouteBuilder {
	b.route = NewRouteTypeHash(funcID, hashKey, areaID, 1)
	return b
}

// Build finalizes the construction of the route. It validates the configured route
// and returns it, or an error if the configuration is incomplete or invalid.
func (b *RouteBuilder) Build() (Route, error) {
	if b.route == nil {
		return nil, fmt.Errorf("route builder: no route type specified")
	}
	if err := b.route.Validate(); err != nil {
		return nil, err
	}
	return b.route, nil
}

// RouteContext carries contextual information for a routing operation, including
// the route itself and data for distributed tracing and observability.
type RouteContext struct {
	context.Context
	Route     Route
	TraceID   string
	StartTime int64
}

// NewRouteContext creates a new RouteContext.
func NewRouteContext(ctx context.Context, route Route) *RouteContext {
	return &RouteContext{
		Context:   ctx,
		Route:     route,
		TraceID:   generateTraceID(),
		StartTime: time.Now().UnixNano(),
	}
}

// generateTraceID creates a simple, unique identifier for tracing purposes.
// A production system might use a more robust UUID or a dedicated tracing library's format.
func generateTraceID() string {
	return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), rand.Int63())
}
