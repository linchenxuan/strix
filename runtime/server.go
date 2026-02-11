// Package runtime provides functionality related to the server's core runtime environment,
// including the management of the server's unique identity (EntityID) and versioning.
package runtime

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Global variables storing the initialized identity of this server instance.
var (
	_srcEntityID         uint32        // The packed 32-bit entity ID of this server.
	_srcEntityIDStr      string        // The string representation (e.g., "1.0.10.1") of the entity ID.
	_areaID              int           // The parsed Area ID.
	_setID               int           // The parsed Set ID.
	_funcID              int           // The parsed Function ID.
	_instID              int           // The parsed Instance ID.
	_frontendEntityID    uint32        // The packed entity ID of the associated frontend server.
	_frontendEntityIDStr string        // The string representation of the frontend entity ID.
	_buildTimeStr        string        // The server's build time as a string, used as a fallback version.
	_buildTime           uint32        // The server's build time as a UNIX timestamp.
	_setVerion           atomic.Uint64 // The version of the server's set, for routing and deployment.
	_svrVerion           atomic.Uint32 // The specific version of this server binary.
)

// EntityID Structure and Bit Offsets
// An EntityID is a 32-bit integer that uniquely identifies a server process within the distributed cluster.
// It is composed of four parts, packed into the integer using bit fields.
//
// The structure (from most significant bit to least significant bit) is:
// [ 5 bits: AreaID | 4 bits: SetID | 8 bits: FuncID | 15 bits: InstID ]
//
// Example: An entity ID string "1.0.10.1" translates to:
// - AreaID: 1
// - SetID:  0
// - FuncID: 10
// - InstID: 1
const (
	// FuncIDOffset is the bit shift required to position the FuncID.
	FuncIDOffset = 15
	// SetIDOffset is the bit shift required to position the SetID.
	SetIDOffset = 23
	// AreaIDOffset is the bit shift required to position the AreaID.
	AreaIDOffset = 27
)

// Bitmasks for extracting components from a packed EntityID.
const (
	_instIDMask = 0x00007FFF // 15 bits for the instance ID.
	_funcIDMask = 0x000000FF // 8 bits for the function ID.
	_setIDMask  = 0x0000000F // 4 bits for the set ID.
	_areaIDMask = 0x0000001F // 5 bits for the area ID.
)

// GetEntityIDByStr parses a dot-decimal entity ID string (e.g., "1.0.10.1") into its
// constituent parts and a packed 32-bit integer representation.
// The format is expected to be "AreaID.SetID.FuncID.InstID".
func GetEntityIDByStr(entityIDStr string) (packedEntityID uint32, areaID, setID, funcID, instID int, err error) {
	n, err := fmt.Sscanf(entityIDStr, "%d.%d.%d.%d", &areaID, &setID, &funcID, &instID)
	if err != nil || n < 4 {
		return 0, 0, 0, 0, 0, fmt.Errorf("entity ID '%s' has an invalid format", entityIDStr)
	}

	// Validate that each component is within its allowed range.
	if areaID <= 0 || setID < 0 || funcID <= 0 || instID <= 0 {
		return 0, 0, 0, 0, 0, fmt.Errorf("entity ID '%s' contains invalid (zero or negative) components", entityIDStr)
	}
	if areaID > _areaIDMask || setID > _setIDMask || funcID > _funcIDMask || instID > _instIDMask {
		return 0, 0, 0, 0, 0, fmt.Errorf("entity ID '%s' component exceeds its allowed range", entityIDStr)
	}

	// Pack the components into a single 32-bit integer.
	var packedTmp uint32
	packedTmp |= uint32(instID)
	packedTmp |= (uint32(funcID) & _funcIDMask) << FuncIDOffset
	packedTmp |= (uint32(setID) & _setIDMask) << SetIDOffset
	packedTmp |= (uint32(areaID) & _areaIDMask) << AreaIDOffset

	return ConvEndian(packedTmp), areaID, setID, funcID, instID, nil
}

// GetStringByEntityID converts a packed 32-bit entity ID back into its dot-decimal string representation.
func GetStringByEntityID(entityID uint32) string {
	var sb strings.Builder
	sb.Grow(16) // Pre-allocate for performance.
	_, _ = sb.WriteString(strconv.Itoa(int(GetAreaIDByEntityID(entityID))))
	_, _ = sb.WriteString(".")
	_, _ = sb.WriteString(strconv.Itoa(int(GetSetIDByEntityID(entityID))))
	_, _ = sb.WriteString(".")
	_, _ = sb.WriteString(strconv.Itoa(int(GetFuncIDByEntityID(entityID))))
	_, _ = sb.WriteString(".")
	_, _ = sb.WriteString(strconv.Itoa(int(GetInstIDByEntityID(entityID))))
	return sb.String()
}

// GetAreaIDByEntityID extracts the Area ID from a packed 32-bit entity ID.
func GetAreaIDByEntityID(entityID uint32) uint32 {
	entityID = ConvEndian(entityID)
	return (entityID >> AreaIDOffset) & _areaIDMask
}

// GetSetIDByEntityID extracts the Set ID from a packed 32-bit entity ID.
func GetSetIDByEntityID(entityID uint32) uint32 {
	entityID = ConvEndian(entityID)
	return (entityID >> SetIDOffset) & _setIDMask
}

// GetFuncIDByEntityID extracts the Function ID from a packed 32-bit entity ID.
func GetFuncIDByEntityID(entityID uint32) uint32 {
	entityID = ConvEndian(entityID)
	return (entityID >> FuncIDOffset) & _funcIDMask
}

// GetInstIDByEntityID extracts the Instance ID from a packed 32-bit entity ID.
func GetInstIDByEntityID(entityID uint32) uint32 {
	entityID = ConvEndian(entityID)
	return entityID & _instIDMask
}

// ChangeInstIDByEntityID creates a new entity ID by replacing the instance ID part of an existing entity ID.
func ChangeInstIDByEntityID(entityID uint32, newInstID uint32) uint32 {
	entityID = ConvEndian(entityID)
	newInstID &= _instIDMask         // Ensure the new instance ID fits within its mask.
	entityID &= ^uint32(_instIDMask) // Clear the old instance ID bits.
	entityID |= newInstID            // Set the new instance ID bits.
	return ConvEndian(entityID)
}

// ConvEndian swaps the byte order of a 32-bit integer. This is used because the packing logic
// effectively creates a big-endian integer in memory, which may need to be converted depending
// on the system's native endianness or network byte order expectations.
func ConvEndian(entityID uint32) uint32 {
	var tmpSli [4]byte
	binary.LittleEndian.PutUint32(tmpSli[:], entityID)
	return binary.BigEndian.Uint32(tmpSli[:])
}

// SetupServerAddr initializes the current server's global identity variables from an entity ID string.
// This function must be called once at startup.
func SetupServerAddr(entityIDStr string) error {
	var err error
	_srcEntityID, _areaID, _setID, _funcID, _instID, err = GetEntityIDByStr(entityIDStr)
	if err != nil {
		return err
	}
	_srcEntityIDStr = entityIDStr
	return nil
}

// SetupFrontendServerAddr initializes the global identity for the associated frontend server.
func SetupFrontendServerAddr(entityIDStr string) error {
	var err error
	_frontendEntityID, _, _, _, _, err = GetEntityIDByStr(entityIDStr)
	if err != nil {
		return err
	}
	_frontendEntityIDStr = entityIDStr
	return nil
}

// GetFrontendEntityID returns the packed 32-bit entity ID of the associated frontend server.
func GetFrontendEntityID() uint32 {
	return _frontendEntityID
}

// GetEntityID returns the packed 32-bit entity ID of the current server instance.
func GetEntityID() uint32 {
	return _srcEntityID
}

// GetEntityIDStr returns the dot-decimal string representation of the current server's entity ID.
func GetEntityIDStr() string {
	return _srcEntityIDStr
}

// GetAreaID returns the Area ID of the current server instance.
func GetAreaID() int {
	return _areaID
}

// GetSetID returns the Set ID of the current server instance.
func GetSetID() int {
	return _setID
}

// GetFuncID returns the Function ID of the current server instance.
func GetFuncID() int {
	return _funcID
}

// GetInsID returns the Instance ID of the current server instance.
func GetInsID() int {
	return _instID
}

// GetSvrBuildTime returns the server's build time as a UNIX timestamp.
// It parses the build time string on first call and caches the result.
func GetSvrBuildTime() uint32 {
	if _buildTime == 0 && _buildTimeStr != "" {
		tmp, err := time.Parse("2006-01-02 15:04:05", _buildTimeStr)
		if err != nil {
			return 0
		}
		_buildTime = uint32(tmp.Unix())
	}
	return _buildTime
}

// SetSvrVersion sets the server's version number. This should be called once at startup.
// It prevents the version from being set more than once.
func SetSvrVersion(svrVersion uint32) error {
	if svrVersion == 0 {
		return errors.New("server version from config is not set")
	}
	if !_svrVerion.CompareAndSwap(0, svrVersion) {
		return fmt.Errorf("server version has already been set to %d", _svrVerion.Load())
	}
	return nil
}

// GetSvrVersion returns the server's version. It prioritizes the explicitly set version,
// falling back to the server's build time if no version has been set.
func GetSvrVersion() uint32 {
	if strVersion := _svrVerion.Load(); strVersion != 0 {
		return strVersion
	}
	return GetSvrBuildTime()
}

// SetSetVersion sets the version of the server's set. This should be called once at startup.
// It prevents the set version from being set more than once.
func SetSetVersion(setVersion uint64) error {
	if setVersion == 0 {
		return errors.New("set version from config is not set")
	}
	if !_setVerion.CompareAndSwap(0, setVersion) {
		return fmt.Errorf("set version has already been set to %d", _setVerion.Load())
	}
	return nil
}

// GetSetVersion returns the version of the server's set. This value is read atomically
// and is not designed to be reloaded dynamically.
func GetSetVersion() uint64 {
	return _setVerion.Load()
}
