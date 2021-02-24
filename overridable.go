package mergo

// Overridable is an interface for configuration settings objects that can be overridden with environment variables
// If a struct implements Overridabale, the name of each field is run through the GetEnvironmentSetting func
// If an environment variable with the matching name is set, then the cvalue of that variable overrides any other setting
type Overridable interface {
	GetEnvironmentSetting(fieldName string) string
}
