//go:build race

package notifications

// ThreadSanitizer requires substantially more memory before application code
// starts. Ordinary and release builds still install the worker data limit.
const raceInstrumentationEnabled = true
