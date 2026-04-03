package lns

import (
	"fmt"
	"sync"
)

// lnsHandlerRegistry holds the registered LNS handlers
var lnsHandlerRegistry = make(map[LNSType]LNSHandler)
var lnsHandlerMutex sync.RWMutex

// RegisterLNSHandler registers an LNS handler for a specific LNS type
// Call this during init() for each LNS handler
func RegisterLNSHandler(lnsType LNSType, handler LNSHandler) {
	lnsHandlerMutex.Lock()
	defer lnsHandlerMutex.Unlock()
	lnsHandlerRegistry[lnsType] = handler
}

// GetLNSHandler retrieves the LNS handler for a given LNS type
// Returns error if no handler is registered
func GetLNSHandler(lnsType LNSType) (LNSHandler, error) {
	lnsHandlerMutex.RLock()
	defer lnsHandlerMutex.RUnlock()

	handler, ok := lnsHandlerRegistry[lnsType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for LNS type: %s", lnsType)
	}
	return handler, nil
}

// MustGetLNSHandler retrieves the LNS handler or panics
// Use this when you know the LNS type is valid
func MustGetLNSHandler(lnsType LNSType) LNSHandler {
	handler, err := GetLNSHandler(lnsType)
	if err != nil {
		panic(err)
	}
	return handler
}
