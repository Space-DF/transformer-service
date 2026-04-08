package deviceprofile

// _global is the process-wide Component instance, set explicitly via SetGlobal.
var _global *Component

// Global returns the process-wide device profile component.
func Global() *Component { return _global }

// SetGlobal sets the process-wide device profile component.
// Call this in main after creating and populating a ComponentRegistry via RegisterAll.
func SetGlobal(c *Component) { _global = c }
