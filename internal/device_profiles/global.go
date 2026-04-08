package deviceprofile

// _global is the single Component instance shared across the process.
// Initialized eagerly so device packages can call Register() from their own init().
var _global = New()

// Global returns the process-wide device profile component.
func Global() *Component { return _global }
